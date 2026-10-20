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

// sessionDoc is the stored shape of domain.Session. The field list must match domain.Session, which
// it is converted to and from directly.
type sessionDoc struct {
	ID        string     `bson:"_id"`
	UserID    string     `bson:"userId"`
	TokenHash string     `bson:"tokenHash"`
	CreatedAt time.Time  `bson:"createdAt"`
	ExpiresAt time.Time  `bson:"expiresAt"`
	UserAgent string     `bson:"userAgent"`
	IP        string     `bson:"ip"`
	FamilyID  string     `bson:"familyId"`
	UsedAt    *time.Time `bson:"usedAt,omitempty"`
}

func (d sessionDoc) toDomain() domain.Session {
	sess := domain.Session(d)
	sess.CreatedAt = sess.CreatedAt.UTC()
	sess.ExpiresAt = sess.ExpiresAt.UTC()
	sess.UsedAt = utcPtr(sess.UsedAt)
	return sess
}

func (s *Store) CreateSession(ctx context.Context, sess domain.Session) error {
	_, err := s.sessions.InsertOne(ctx, sessionDoc(sess))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

// ConsumeSession marks the session used in a single atomic update, so of two concurrent exchanges of
// the same token exactly one wins; the other sees ErrSessionUsed.
func (s *Store) ConsumeSession(ctx context.Context, tokenHash string, now time.Time) (domain.Session, error) {
	var d sessionDoc
	err := s.sessions.FindOneAndUpdate(ctx,
		bson.D{{Key: "tokenHash", Value: tokenHash}, {Key: "usedAt", Value: nil}}, // nil matches "not set"
		bson.D{{Key: "$set", Value: bson.D{{Key: "usedAt", Value: now}}}},
	).Decode(&d) // returns the document as it was before the update
	if err == nil {
		return d.toDomain(), nil
	}
	if !errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.Session{}, err
	}
	// Either there is no such session, or it was used before.
	err = s.sessions.FindOne(ctx, bson.D{{Key: "tokenHash", Value: tokenHash}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.Session{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, err
	}
	return d.toDomain(), store.ErrSessionUsed
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.sessions.DeleteOne(ctx, bson.D{{Key: "tokenHash", Value: tokenHash}})
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.sessions.DeleteMany(ctx, bson.D{{Key: "userId", Value: userID}})
	return err
}

func (s *Store) DeleteFamily(ctx context.Context, familyID string) error {
	if familyID == "" {
		return nil
	}
	_, err := s.sessions.DeleteMany(ctx, bson.D{{Key: "familyId", Value: familyID}})
	return err
}
