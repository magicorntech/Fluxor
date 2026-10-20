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

// userDoc is the stored shape of domain.User.
type userDoc struct {
	ID                 string     `bson:"_id"`
	Username           string     `bson:"username"`
	Email              string     `bson:"email"`
	DisplayName        string     `bson:"displayName"`
	PasswordHash       string     `bson:"passwordHash"`
	MustChangePassword bool       `bson:"mustChangePassword"`
	Disabled           bool       `bson:"disabled"`
	CreatedAt          time.Time  `bson:"createdAt"`
	CreatedBy          string     `bson:"createdBy"`
	LastLoginAt        *time.Time `bson:"lastLoginAt,omitempty"`
	FailedLogins       int        `bson:"failedLogins"`
	LockedUntil        *time.Time `bson:"lockedUntil,omitempty"`
}

func toUserDoc(u domain.User) userDoc {
	return userDoc{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName,
		PasswordHash: u.PasswordHash, MustChangePassword: u.MustChangePassword, Disabled: u.Disabled,
		CreatedAt: u.CreatedAt, CreatedBy: u.CreatedBy, LastLoginAt: u.LastLoginAt,
		FailedLogins: u.FailedLogins, LockedUntil: u.LockedUntil,
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (d userDoc) toDomain() domain.User {
	return domain.User{
		ID: d.ID, Username: d.Username, Email: d.Email, DisplayName: d.DisplayName,
		PasswordHash: d.PasswordHash, MustChangePassword: d.MustChangePassword, Disabled: d.Disabled,
		CreatedAt: d.CreatedAt.UTC(), CreatedBy: d.CreatedBy, LastLoginAt: utcPtr(d.LastLoginAt),
		FailedLogins: d.FailedLogins, LockedUntil: utcPtr(d.LockedUntil),
	}
}

func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	return s.users.CountDocuments(ctx, bson.D{})
}

func (s *Store) CreateUser(ctx context.Context, u domain.User) error {
	_, err := s.users.InsertOne(ctx, toUserDoc(u))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) findUser(ctx context.Context, filter bson.D) (domain.User, error) {
	var d userDoc
	err := s.users.FindOne(ctx, filter).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.User{}, store.ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) GetUser(ctx context.Context, id string) (domain.User, error) {
	return s.findUser(ctx, bson.D{{Key: "_id", Value: id}})
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	return s.findUser(ctx, bson.D{{Key: "username", Value: username}})
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	cur, err := s.users.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "username", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []userDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) UpdateUser(ctx context.Context, u domain.User) error {
	res, err := s.users.ReplaceOne(ctx, bson.D{{Key: "_id", Value: u.ID}}, toUserDoc(u))
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
