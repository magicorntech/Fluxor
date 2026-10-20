package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// auditDoc is the stored shape of domain.AuditEvent.
type auditDoc struct {
	ID          string            `bson:"_id"`
	Time        time.Time         `bson:"time"`
	ActorID     string            `bson:"actorId"`
	ActorName   string            `bson:"actorName"`
	Action      string            `bson:"action"`
	Subject     string            `bson:"subject"`
	SubjectName string            `bson:"subjectName"`
	Result      string            `bson:"result"`
	Status      int               `bson:"status"`
	IP          string            `bson:"ip"`
	Details     map[string]string `bson:"details"`
}

func (s *Store) AddAudit(ctx context.Context, e domain.AuditEvent) error {
	details := e.Details
	if details == nil {
		details = map[string]string{}
	}
	_, err := s.audit.InsertOne(ctx, auditDoc{
		ID: e.ID, Time: e.Time, ActorID: e.ActorID, ActorName: e.ActorName, Action: e.Action,
		Subject: e.Subject, SubjectName: e.SubjectName, Result: string(e.Result), Status: e.Status,
		IP: e.IP, Details: details,
	})
	return err
}

func (s *Store) ListAudit(ctx context.Context, f store.AuditFilter) ([]domain.AuditEvent, error) {
	filter := bson.D{}
	if f.ActorID != "" {
		filter = append(filter, bson.E{Key: "actorId", Value: f.ActorID})
	}
	if f.Action != "" {
		filter = append(filter, bson.E{Key: "action", Value: f.Action})
	}
	if f.Result != "" {
		filter = append(filter, bson.E{Key: "result", Value: string(f.Result)})
	}
	if !f.From.IsZero() || !f.To.IsZero() {
		rng := bson.D{}
		if !f.From.IsZero() {
			rng = append(rng, bson.E{Key: "$gte", Value: f.From})
		}
		if !f.To.IsZero() {
			rng = append(rng, bson.E{Key: "$lt", Value: f.To})
		}
		filter = append(filter, bson.E{Key: "time", Value: rng})
	}
	if c := f.Before; c != nil {
		// Strictly after the cursor in newest-first order: older, or the same instant with a smaller ID.
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "time", Value: bson.D{{Key: "$lt", Value: c.Time}}}},
			bson.D{{Key: "time", Value: c.Time}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: c.ID}}}},
		}})
	}
	limit := int64(f.Limit)
	if limit <= 0 {
		limit = 100
	}
	cur, err := s.audit.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "time", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	var docs []auditDoc
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]domain.AuditEvent, 0, len(docs))
	for _, d := range docs {
		details := d.Details
		if details == nil {
			details = map[string]string{}
		}
		out = append(out, domain.AuditEvent{
			ID: d.ID, Time: d.Time.UTC(), ActorID: d.ActorID, ActorName: d.ActorName, Action: d.Action,
			Subject: d.Subject, SubjectName: d.SubjectName, Result: domain.AuditResult(d.Result), Status: d.Status,
			IP: d.IP, Details: details,
		})
	}
	return out, nil
}
