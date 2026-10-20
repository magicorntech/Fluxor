package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// tokenDoc is the stored shape of domain.APIToken.
type tokenDoc struct {
	ID         string     `bson:"_id"`
	UserID     string     `bson:"userId"`
	Name       string     `bson:"name"`
	Prefix     string     `bson:"prefix"`
	TokenHash  string     `bson:"tokenHash"`
	CreatedAt  time.Time  `bson:"createdAt"`
	ExpiresAt  time.Time  `bson:"expiresAt"`
	LastUsedAt *time.Time `bson:"lastUsedAt,omitempty"`
}

func toTokenDoc(t domain.APIToken) tokenDoc {
	return tokenDoc{
		ID: t.ID, UserID: t.UserID, Name: t.Name, Prefix: t.Prefix, TokenHash: t.TokenHash,
		CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt,
	}
}

func (d tokenDoc) toDomain() domain.APIToken {
	return domain.APIToken{
		ID: d.ID, UserID: d.UserID, Name: d.Name, Prefix: d.Prefix, TokenHash: d.TokenHash,
		CreatedAt: d.CreatedAt.UTC(), ExpiresAt: d.ExpiresAt.UTC(), LastUsedAt: utcPtr(d.LastUsedAt),
	}
}

func (s *Store) CreateToken(ctx context.Context, t domain.APIToken) error {
	_, err := s.tokens.InsertOne(ctx, toTokenDoc(t))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) GetTokenByHash(ctx context.Context, hash string) (domain.APIToken, error) {
	var d tokenDoc
	err := s.tokens.FindOne(ctx, bson.D{{Key: "tokenHash", Value: hash}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.APIToken{}, store.ErrNotFound
	}
	if err != nil {
		return domain.APIToken{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) ListTokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	cur, err := s.tokens.Find(ctx, bson.D{{Key: "userId", Value: userID}},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var docs []tokenDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.APIToken, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) CountTokens(ctx context.Context, userID string) (int64, error) {
	return s.tokens.CountDocuments(ctx, bson.D{{Key: "userId", Value: userID}})
}

func (s *Store) DeleteToken(ctx context.Context, userID, id string) error {
	res, err := s.tokens.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "userId", Value: userID}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUserTokens(ctx context.Context, userID string) error {
	_, err := s.tokens.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}})
	return err
}

func (s *Store) TouchToken(ctx context.Context, id string, at time.Time) error {
	res, err := s.tokens.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "lastUsedAt", Value: at}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}
