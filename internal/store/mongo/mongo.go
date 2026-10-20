// Package mongo is the MongoDB implementation of the store interfaces.
package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/magicorntech/Fluxor/internal/store"
)

type Store struct {
	client   *mongodriver.Client
	db       *mongodriver.Database
	clusters *mongodriver.Collection
	users    *mongodriver.Collection
	sessions *mongodriver.Collection
	bindings *mongodriver.Collection
	teams    *mongodriver.Collection
	groups   *mongodriver.Collection
	tokens   *mongodriver.Collection
	audit    *mongodriver.Collection
}

var _ store.Store = (*Store)(nil)

// Connect opens a connection, verifies it with a ping and ensures all indexes exist.
// The context bounds the ping and index creation, not the lifetime of the client.
func Connect(ctx context.Context, uri, dbName string) (*Store, error) {
	client, err := mongodriver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	db := client.Database(dbName)
	s := &Store{
		client:   client,
		db:       db,
		clusters: db.Collection("clusters"),
		users:    db.Collection("users"),
		sessions: db.Collection("sessions"),
		bindings: db.Collection("role_bindings"),
		teams:    db.Collection("teams"),
		groups:   db.Collection("resource_groups"),
		tokens:   db.Collection("api_tokens"),
		audit:    db.Collection("audit_events"),
	}
	if err := s.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return s, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.client.Disconnect(ctx)
}

// DropDatabase removes the whole database. Intended for tests only.
func (s *Store) DropDatabase(ctx context.Context) error {
	return s.db.Drop(ctx)
}

func (s *Store) ensureIndexes(ctx context.Context) error {
	indexes := []struct {
		coll   *mongodriver.Collection
		models []mongodriver.IndexModel
	}{
		{s.clusters, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true).SetName("name_unique")},
		}},
		{s.users, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true).SetName("username_unique")},
			// Unique only among users that have an email; users without one are not indexed.
			{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).SetName("email_unique").
				SetPartialFilterExpression(bson.D{{Key: "email", Value: bson.D{{Key: "$gt", Value: ""}}}})},
		}},
		{s.sessions, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true).SetName("token_hash_unique")},
			{Keys: bson.D{{Key: "userId", Value: 1}}, Options: options.Index().SetName("user_id")},
			{Keys: bson.D{{Key: "familyId", Value: 1}}, Options: options.Index().SetName("family_id")},
			// Mongo removes expired sessions on its own (the sweep runs about once a minute).
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0).SetName("expires_at_ttl")},
		}},
		{s.bindings, []mongodriver.IndexModel{
			// key encodes subject + role + scope, so the same grant cannot be stored twice.
			{Keys: bson.D{{Key: "key", Value: 1}}, Options: options.Index().SetUnique(true).SetName("key_unique")},
			{Keys: bson.D{{Key: "subjectKind", Value: 1}, {Key: "subjectId", Value: 1}}, Options: options.Index().SetName("subject")},
			{Keys: bson.D{{Key: "roleId", Value: 1}}, Options: options.Index().SetName("role_id")},
		}},
		{s.teams, []mongodriver.IndexModel{
			// Names are unique ignoring case (nameKey is the lowercase name).
			{Keys: bson.D{{Key: "nameKey", Value: 1}}, Options: options.Index().SetUnique(true).SetName("name_key_unique")},
			// Multikey index: finds the teams of a user.
			{Keys: bson.D{{Key: "members", Value: 1}}, Options: options.Index().SetName("members")},
		}},
		{s.groups, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "nameKey", Value: 1}}, Options: options.Index().SetUnique(true).SetName("kind_name_key_unique")},
			{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "members", Value: 1}}, Options: options.Index().SetName("kind_members")},
		}},
		{s.tokens, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true).SetName("token_hash_unique")},
			{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}}, Options: options.Index().SetName("user_created")},
			// Expired tokens are removed by Mongo on its own (the sweep runs about once a minute).
			{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0).SetName("expires_at_ttl")},
		}},
		{s.audit, []mongodriver.IndexModel{
			{Keys: bson.D{{Key: "time", Value: -1}, {Key: "_id", Value: -1}}, Options: options.Index().SetName("time_id")},
			{Keys: bson.D{{Key: "actorId", Value: 1}, {Key: "time", Value: -1}}, Options: options.Index().SetName("actor_time")},
			{Keys: bson.D{{Key: "action", Value: 1}, {Key: "time", Value: -1}}, Options: options.Index().SetName("action_time")},
		}},
	}
	for _, ix := range indexes {
		if _, err := ix.coll.Indexes().CreateMany(ctx, ix.models); err != nil {
			return fmt.Errorf("create %s indexes: %w", ix.coll.Name(), err)
		}
	}
	return nil
}
