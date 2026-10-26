package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/jingjoeh/test-assignment/user-management/internal/domain"
	"github.com/jingjoeh/test-assignment/user-management/internal/port"
)

// CollectionName is the collection that holds users.
const CollectionName = "users"

// userDocument is the BSON shape of a user. It stays inside this package, so
// ObjectID never leaks into the domain (D15).
type userDocument struct {
	ID           bson.ObjectID `bson:"_id"`
	Name         string        `bson:"name"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    time.Time     `bson:"created_at"`
}

// UserRepository implements port.UserRepository on a MongoDB collection.
type UserRepository struct {
	coll    *mongo.Collection
	timeout time.Duration
}

var _ port.UserRepository = (*UserRepository)(nil)

// NewUserRepository returns a repository on db's users collection. Every call
// is bounded by timeout on top of the caller's context.
func NewUserRepository(db *mongo.Database, timeout time.Duration) *UserRepository {
	return &UserRepository{coll: db.Collection(CollectionName), timeout: timeout}
}

// EnsureIndexes creates the unique index on email. It is idempotent and is
// called once at startup.
func (r *UserRepository) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("email_unique"),
	})
	if err != nil {
		return fmt.Errorf("mongo: create email index: %w", err)
	}
	return nil
}

func (r *UserRepository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	doc := toDocument(u)
	doc.ID = bson.NewObjectID()
	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		return domain.User{}, mapError("insert user", err)
	}
	return fromDocument(doc), nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	oid, err := parseID(id)
	if err != nil {
		return domain.User{}, err
	}
	return r.findOne(ctx, bson.D{{Key: "_id", Value: oid}})
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findOne(ctx, bson.D{{Key: "email", Value: email}})
}

func (r *UserRepository) findOne(ctx context.Context, filter bson.D) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var doc userDocument
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		return domain.User{}, mapError("find user", err)
	}
	return fromDocument(doc), nil
}

func (r *UserRepository) List(ctx context.Context) ([]domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// ObjectIDs start with a timestamp, so sorting by _id is oldest first.
	cur, err := r.coll.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, mapError("list users", err)
	}
	var docs []userDocument
	if err := cur.All(ctx, &docs); err != nil {
		return nil, mapError("list users", err)
	}

	users := make([]domain.User, 0, len(docs))
	for _, d := range docs {
		users = append(users, fromDocument(d))
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, id string, upd port.UserUpdate) (domain.User, error) {
	oid, err := parseID(id)
	if err != nil {
		return domain.User{}, err
	}

	set := bson.D{}
	if upd.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *upd.Name})
	}
	if upd.Email != nil {
		set = append(set, bson.E{Key: "email", Value: *upd.Email})
	}
	if len(set) == 0 {
		// Nothing to change; the service rejects this case, but stay correct anyway.
		return r.GetByID(ctx, id)
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	var doc userDocument
	err = r.coll.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: oid}},
		bson.D{{Key: "$set", Value: set}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&doc)
	if err != nil {
		return domain.User{}, mapError("update user", err)
	}
	return fromDocument(doc), nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	oid, err := parseID(id)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	res, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: oid}})
	if err != nil {
		return mapError("delete user", err)
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	n, err := r.coll.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, mapError("count users", err)
	}
	return n, nil
}

// parseID turns a hex string into an ObjectID. A malformed ID is a client
// error (400), not a missing user (404) (D15).
func parseID(id string) (bson.ObjectID, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.ObjectID{}, domain.ErrInvalidID
	}
	return oid, nil
}

// mapError translates driver errors into domain errors. Anything it doesn't
// recognize is wrapped and treated as an infrastructure failure.
func mapError(op string, err error) error {
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return domain.ErrNotFound
	case mongo.IsDuplicateKeyError(err):
		return domain.ErrEmailExists
	default:
		return fmt.Errorf("mongo: %s: %w", op, err)
	}
}

func toDocument(u domain.User) userDocument {
	return userDocument{
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		// BSON dates have millisecond precision; truncate so the value we
		// return from Create matches what a later read returns.
		CreatedAt: u.CreatedAt.UTC().Truncate(time.Millisecond),
	}
}

func fromDocument(d userDocument) domain.User {
	return domain.User{
		ID:           d.ID.Hex(),
		Name:         d.Name,
		Email:        d.Email,
		PasswordHash: d.PasswordHash,
		CreatedAt:    d.CreatedAt.UTC(),
	}
}
