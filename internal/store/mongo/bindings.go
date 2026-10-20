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

// bindingDoc is the stored shape of domain.Binding, flattened so every field is indexable.
type bindingDoc struct {
	ID          string    `bson:"_id"`
	Key         string    `bson:"key"`
	SubjectKind string    `bson:"subjectKind"`
	SubjectID   string    `bson:"subjectId"`
	RoleID      string    `bson:"roleId"`
	ClusterKind string    `bson:"clusterKind"`
	ClusterID   string    `bson:"clusterId"`
	AppKind     string    `bson:"appKind"`
	AppID       string    `bson:"appId"`
	CreatedAt   time.Time `bson:"createdAt"`
	CreatedBy   string    `bson:"createdBy"`
}

func toBindingDoc(b domain.Binding) bindingDoc {
	return bindingDoc{
		ID: b.ID, Key: b.Key(),
		SubjectKind: string(b.Subject.Kind), SubjectID: b.Subject.ID,
		RoleID:      b.RoleID,
		ClusterKind: string(b.Scope.Cluster.Kind), ClusterID: b.Scope.Cluster.ID,
		AppKind: string(b.Scope.Application.Kind), AppID: b.Scope.Application.ID,
		CreatedAt: b.CreatedAt, CreatedBy: b.CreatedBy,
	}
}

func (d bindingDoc) toDomain() domain.Binding {
	return domain.Binding{
		ID:      d.ID,
		Subject: domain.Subject{Kind: domain.SubjectKind(d.SubjectKind), ID: d.SubjectID},
		RoleID:  d.RoleID,
		Scope: domain.Scope{
			Cluster:     domain.Ref{Kind: domain.RefKind(d.ClusterKind), ID: d.ClusterID},
			Application: domain.Ref{Kind: domain.RefKind(d.AppKind), ID: d.AppID},
		},
		CreatedAt: d.CreatedAt.UTC(),
		CreatedBy: d.CreatedBy,
	}
}

func (s *Store) CreateBinding(ctx context.Context, b domain.Binding) error {
	_, err := s.bindings.InsertOne(ctx, toBindingDoc(b))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) GetBinding(ctx context.Context, id string) (domain.Binding, error) {
	var d bindingDoc
	err := s.bindings.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.Binding{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Binding{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) ListBindings(ctx context.Context, f store.BindingFilter) ([]domain.Binding, error) {
	filter := bson.D{}
	if f.RoleID != "" {
		filter = append(filter, bson.E{Key: "roleId", Value: f.RoleID})
	}
	if len(f.Subjects) > 0 {
		subjects := make(bson.A, 0, len(f.Subjects))
		for _, sub := range f.Subjects {
			subjects = append(subjects, bson.D{{Key: "subjectKind", Value: string(sub.Kind)}, {Key: "subjectId", Value: sub.ID}})
		}
		filter = append(filter, bson.E{Key: "$or", Value: subjects})
	}
	cur, err := s.bindings.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []bindingDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.Binding, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) DeleteBinding(ctx context.Context, id string) error {
	res, err := s.bindings.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}
