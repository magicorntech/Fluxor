package mongo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// groupDoc is the stored shape of domain.ResourceGroup.
type groupDoc struct {
	ID          string    `bson:"_id"`
	Kind        string    `bson:"kind"`
	Name        string    `bson:"name"`
	NameKey     string    `bson:"nameKey"`
	Description string    `bson:"description"`
	Members     []string  `bson:"members"`
	CreatedAt   time.Time `bson:"createdAt"`
	CreatedBy   string    `bson:"createdBy"`
}

func toGroupDoc(g domain.ResourceGroup) groupDoc {
	return groupDoc{
		ID: g.ID, Kind: string(g.Kind), Name: g.Name, NameKey: domain.NameKey(g.Name), Description: g.Description,
		Members: nonNil(g.Members), CreatedAt: g.CreatedAt, CreatedBy: g.CreatedBy,
	}
}

func (d groupDoc) toDomain() domain.ResourceGroup {
	return domain.ResourceGroup{
		ID: d.ID, Kind: domain.GroupKind(d.Kind), Name: d.Name, Description: d.Description,
		Members: nonNil(d.Members), CreatedAt: d.CreatedAt.UTC(), CreatedBy: d.CreatedBy,
	}
}

func (s *Store) CreateGroup(ctx context.Context, g domain.ResourceGroup) error {
	_, err := s.groups.InsertOne(ctx, toGroupDoc(g))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) GetGroup(ctx context.Context, id string) (domain.ResourceGroup, error) {
	var d groupDoc
	err := s.groups.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.ResourceGroup{}, store.ErrNotFound
	}
	if err != nil {
		return domain.ResourceGroup{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) ListGroups(ctx context.Context, f store.GroupFilter) ([]domain.ResourceGroup, error) {
	filter := bson.D{}
	if f.Kind != "" {
		filter = append(filter, bson.E{Key: "kind", Value: string(f.Kind)})
	}
	if len(f.IDs) > 0 {
		filter = append(filter, bson.E{Key: "_id", Value: bson.D{{Key: "$in", Value: f.IDs}}})
	}
	cur, err := s.groups.Find(ctx, filter, byName)
	if err != nil {
		return nil, err
	}
	var docs []groupDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.ResourceGroup, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) UpdateGroup(ctx context.Context, g domain.ResourceGroup) error {
	res, err := s.groups.UpdateOne(ctx, bson.D{{Key: "_id", Value: g.ID}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: g.Name},
		{Key: "nameKey", Value: domain.NameKey(g.Name)},
		{Key: "description", Value: g.Description},
	}}})
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) SetGroupMembers(ctx context.Context, id string, members []string) error {
	res, err := s.groups.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "members", Value: nonNil(members)}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	res, err := s.groups.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) RemoveFromGroups(ctx context.Context, kind domain.GroupKind, resourceID string) error {
	_, err := s.groups.UpdateMany(ctx,
		bson.D{{Key: "kind", Value: string(kind)}, {Key: "members", Value: resourceID}},
		bson.D{{Key: "$pull", Value: bson.D{{Key: "members", Value: resourceID}}}})
	return err
}
