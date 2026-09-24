package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gbexam/online-exam/internal/constants"
	"github.com/gbexam/online-exam/internal/model"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.WrongQuestion{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM wrong_questions")
	})
	return NewRepository(db)
}

func TestUpsertWrongQuestionSameAttemptDoesNotDoubleCount(t *testing.T) {
	r := newTestRepository(t)
	ctx := context.Background()
	now := time.Now()
	base := func() *model.WrongQuestion {
		return &model.WrongQuestion{
			StudentID: 1, QuestionID: 9, KnowledgePoint: "语文",
			WrongCount: 1, LostScore: 4, LastAttemptID: 100,
			LastWrongAt: now, Status: constants.WrongUnresolved,
		}
	}

	if err := r.UpsertWrongQuestion(ctx, base()); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Teacher re-saves the same partial score for the same attempt.
	if err := r.UpsertWrongQuestion(ctx, base()); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	rec, err := r.FindWrongQuestion(ctx, 1, 9)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if rec.WrongCount != 1 {
		t.Fatalf("wrong count should remain 1 for same attempt, got %d", rec.WrongCount)
	}
	if rec.LostScore != 4 || rec.LastAttemptID != 100 {
		t.Fatalf("unexpected record: %+v", rec)
	}

	// A new attempt losing points on the same question counts again.
	other := base()
	other.LostScore = 2
	other.LastAttemptID = 101
	if err := r.UpsertWrongQuestion(ctx, other); err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	rec, _ = r.FindWrongQuestion(ctx, 1, 9)
	if rec.WrongCount != 2 || rec.LostScore != 2 {
		t.Fatalf("expected count 2 lost 2, got count %d lost %v", rec.WrongCount, rec.LostScore)
	}
}

func TestResolveWrongQuestionByQuestion(t *testing.T) {
	r := newTestRepository(t)
	ctx := context.Background()
	w := &model.WrongQuestion{
		StudentID: 1, QuestionID: 5, WrongCount: 1,
		LastAttemptID: 7, LastWrongAt: time.Now(), Status: constants.WrongUnresolved,
	}
	if err := r.UpsertWrongQuestion(ctx, w); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := r.ResolveWrongQuestionByQuestion(ctx, 1, 5); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rec, _ := r.FindWrongQuestion(ctx, 1, 5)
	if rec.Status != constants.WrongResolved {
		t.Fatalf("expected resolved, got %q", rec.Status)
	}

	// No record for another question: must be a silent no-op.
	if err := r.ResolveWrongQuestionByQuestion(ctx, 1, 999); err != nil {
		t.Fatalf("resolve missing record should be no-op, got %v", err)
	}
}
