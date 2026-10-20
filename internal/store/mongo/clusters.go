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

// clusterDoc is the stored shape of domain.Cluster.
type clusterDoc struct {
	ID        string            `bson:"_id"`
	Name      string            `bson:"name"`
	APIServer string            `bson:"apiServer"`
	Labels    map[string]string `bson:"labels"`
	Status    string            `bson:"status"`
	Version   string            `bson:"version"`
	CreatedAt time.Time         `bson:"createdAt"`
}

func toClusterDoc(c domain.Cluster) clusterDoc {
	return clusterDoc{
		ID:        c.ID,
		Name:      c.Name,
		APIServer: c.APIServer,
		Labels:    c.Labels,
		Status:    string(c.Status),
		Version:   c.Version,
		CreatedAt: c.CreatedAt,
	}
}

func (d clusterDoc) toDomain() domain.Cluster {
	labels := d.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return domain.Cluster{
		ID:        d.ID,
		Name:      d.Name,
		APIServer: d.APIServer,
		Labels:    labels,
		Status:    domain.ClusterStatus(d.Status),
		Version:   d.Version,
		CreatedAt: d.CreatedAt.UTC(),
	}
}

func (s *Store) ListClusters(ctx context.Context) ([]domain.Cluster, error) {
	cur, err := s.clusters.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var docs []clusterDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.Cluster, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.toDomain())
	}
	return out, nil
}

func (s *Store) GetCluster(ctx context.Context, id string) (domain.Cluster, error) {
	var d clusterDoc
	err := s.clusters.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&d)
	if errors.Is(err, mongodriver.ErrNoDocuments) {
		return domain.Cluster{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Cluster{}, err
	}
	return d.toDomain(), nil
}

func (s *Store) CreateCluster(ctx context.Context, c domain.Cluster) error {
	_, err := s.clusters.InsertOne(ctx, toClusterDoc(c))
	if mongodriver.IsDuplicateKeyError(err) {
		return store.ErrAlreadyExists
	}
	return err
}

func (s *Store) DeleteCluster(ctx context.Context, id string) error {
	res, err := s.clusters.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}
