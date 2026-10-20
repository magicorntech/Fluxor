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

// teamDoc is the stored shape of domain.Team. nameKey is the lowercase name, kept for the
// case-insensitive unique index.
type teamDoc struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	NameKey     string    `bson:"nameKey"`
	Description string    `bson:"description"`
	Members     []string  `bson:"members"`
	CreatedAt   time.Time `bson:"createdAt"`
	CreatedBy   string    `bson:"createdBy"`
}

func toTeamDoc(t domain.Team) teamDoc {
	return teamDoc{
		ID: t.ID, Name: t.Name, NameKey: domain.NameKey(t.Name), Description: t.Description,
		Members: nonNil(t.Members), CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy,
	}
}

func (d teamDoc) toDomain() domain.Team {
	return domain.Team{
		ID: d.ID, Name: d.Name, Description: d.Description, Members: nonNil(d.Members),
		CreatedAt: d.CreatedAt.UTC(), CreatedBy: d.CreatedBy,
	}
}

// nonNil makes sure an empty list is stored and returned as [] rather than null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

var byName = options.Find().SetSort(bson.D{{Key: "nameKey", Value: 1}, {Key: "_id", Value: 1}})

func (s *Store) CreateTeam(ctx context.Context, t domain.Team) error {
	_, err := s.teams.InsertOne(ctx, toTeamDoc(t))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) GetTeam(ctx context.Context, id string) (domain.Team, error) {
	var d teamDoc
	err := s.teams.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.Team{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Team{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) findTeams(ctx context.Context, filter bson.D) ([]domain.Team, error) {
	cur, err := s.teams.Find(ctx, filter, byName)
	if err != nil {
		return nil, err
	}
	var docs []teamDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.Team, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) ListTeams(ctx context.Context) ([]domain.Team, error) {
	return s.findTeams(ctx, bson.D{})
}

func (s *Store) ListTeamsOfUser(ctx context.Context, userID string) ([]domain.Team, error) {
	return s.findTeams(ctx, bson.D{{Key: "members", Value: userID}})
}

func (s *Store) UpdateTeam(ctx context.Context, t domain.Team) error {
	res, err := s.teams.UpdateOne(ctx, bson.D{{Key: "_id", Value: t.ID}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "name", Value: t.Name},
		{Key: "nameKey", Value: domain.NameKey(t.Name)},
		{Key: "description", Value: t.Description},
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

func (s *Store) SetTeamMembers(ctx context.Context, id string, members []string) error {
	res, err := s.teams.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "members", Value: nonNil(members)}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	res, err := s.teams.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}
