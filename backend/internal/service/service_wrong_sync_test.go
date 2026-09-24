package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/gbexam/online-exam/internal/model"
)

// fakeWrongRepo is an in-memory WrongRepo for testing the wrong-book sync rules.
type fakeWrongRepo struct {
	records map[string]*model.WrongQuestion
}

func newFakeWrongRepo() *fakeWrongRepo {
	return &fakeWrongRepo{records: map[string]*model.WrongQuestion{}}
}

func (f *fakeWrongRepo) key(studentID, questionID uint) string {
	return fmt.Sprintf("%d:%d", studentID, questionID)
}

func (f *fakeWrongRepo) UpsertWrongQuestion(_ context.Context, w *model.WrongQuestion) error {
	k := f.key(w.StudentID, w.QuestionID)
	if existing, ok := f.records[k]; ok {
		existing.WrongCount++
		existing.LostScore = w.LostScore
		existing.LastWrongAt = w.LastWrongAt
		existing.Status = w.Status
		w.ID = existing.ID
		return nil
	}
	w.ID = uint(len(f.records) + 1)
	cp := *w
	f.records[k] = &cp
	return nil
}

func (f *fakeWrongRepo) RecordSubjectiveWrong(_ context.Context, w *model.WrongQuestion) error {
	k := f.key(w.StudentID, w.QuestionID)
	if existing, ok := f.records[k]; ok {
		if existing.LostScore != w.LostScore {
			existing.WrongCount++
		}
		existing.LostScore = w.LostScore
		existing.LastWrongAt = w.LastWrongAt
		existing.Status = w.Status
		w.ID = existing.ID
		return nil
	}
	w.ID = uint(len(f.records) + 1)
	cp := *w
	f.records[k] = &cp
	return nil
}

func (f *fakeWrongRepo) ResolveWrongQuestionByQuestion(_ context.Context, studentID, questionID uint) error {
	if existing, ok := f.records[f.key(studentID, questionID)]; ok {
		existing.Status = "resolved"
	}
	return nil
}

func (f *fakeWrongRepo) MarkWrongQuestionResolved(_ context.Context, id, _ uint) error {
	for _, r := range f.records {
		if r.ID == id {
			r.Status = "resolved"
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (f *fakeWrongRepo) ListWrongQuestions(_ context.Context, studentID uint, _ string, _, _ int) ([]model.WrongQuestion, int64, error) {
	items := make([]model.WrongQuestion, 0)
	for _, r := range f.records {
		if r.StudentID == studentID {
			items = append(items, *r)
		}
	}
	return items, int64(len(items)), nil
}

func (f *fakeWrongRepo) DeleteWrongQuestion(_ context.Context, _, _ uint) error { return nil }

// TestRecordSubjectiveWrongIdempotent verifies that repeatedly recording the
// same low score does not inflate the wrong count, while a changed score does.
func TestRecordSubjectiveWrongIdempotent(t *testing.T) {
	repo := newFakeWrongRepo()
	svc := &AttemptService{wrongRepo: repo}
	ctx := context.Background()
	q := &model.Question{ID: 7, KnowledgePoint: "函数", Score: 10}

	record := func(score float64, gradedBy uint, prevScore float64) error {
		answer := &model.Answer{Score: prevScore, GradedBy: gradedBy}
		return svc.syncSubjectiveWrongBook(ctx, 1, q, 10, score, answer)
	}

	// First grading: 6/10 -> enters the unresolved book, one error.
	if err := record(6, 0, 0); err != nil {
		t.Fatalf("first grade: %v", err)
	}
	r := repo.records[repo.key(1, 7)]
	if r.Status != "unresolved" || r.WrongCount != 1 || r.LostScore != 4 {
		t.Fatalf("unexpected record after first grade: %+v", r)
	}

	// Save the same 6/10 again (teacher re-saves) -> count must not increase.
	if err := record(6, 42, 6); err != nil {
		t.Fatalf("same score re-save: %v", err)
	}
	if r.WrongCount != 1 || r.LostScore != 4 || r.Status != "unresolved" {
		t.Fatalf("same low score should not change the record: %+v", r)
	}

	// Teacher changes the score to 8/10 -> new lost score, count increases.
	if err := record(8, 42, 6); err != nil {
		t.Fatalf("changed score: %v", err)
	}
	if r.WrongCount != 2 || r.LostScore != 2 || r.Status != "unresolved" {
		t.Fatalf("changed low score should increment the count: %+v", r)
	}

	// Teacher re-grades to full marks -> mastered, leaves practice.
	if err := record(10, 42, 8); err != nil {
		t.Fatalf("full score: %v", err)
	}
	if r.Status != "resolved" {
		t.Fatalf("full score should resolve the wrong question: %+v", r)
	}

	// Saving full marks again changes nothing.
	if err := record(10, 42, 10); err != nil {
		t.Fatalf("full score re-save: %v", err)
	}
	if r.Status != "resolved" || r.WrongCount != 2 {
		t.Fatalf("repeated full score should keep the record untouched: %+v", r)
	}

	// Teacher lowers it back to 5/10 after full marks -> unresolved again.
	if err := record(5, 42, 10); err != nil {
		t.Fatalf("re-grade low: %v", err)
	}
	if r.Status != "unresolved" || r.WrongCount != 3 || r.LostScore != 5 {
		t.Fatalf("lowering after full marks should reopen the record: %+v", r)
	}
}

// TestRecordSubjectiveWrongZeroScore verifies a first-time 0 score is still a
// wrong answer even though 0 equals the ungraded default score.
func TestRecordSubjectiveWrongZeroScore(t *testing.T) {
	repo := newFakeWrongRepo()
	svc := &AttemptService{wrongRepo: repo}
	ctx := context.Background()
	q := &model.Question{ID: 9, KnowledgePoint: "集合", Score: 5}

	answer := &model.Answer{Score: 0, GradedBy: 0}
	if err := svc.syncSubjectiveWrongBook(ctx, 2, q, 5, 0, answer); err != nil {
		t.Fatalf("zero score grade: %v", err)
	}
	r := repo.records[repo.key(2, 9)]
	if r == nil || r.Status != "unresolved" || r.WrongCount != 1 || r.LostScore != 5 {
		t.Fatalf("zero score must create an unresolved record with full lost score, got %+v", r)
	}
}

// Ensure the fake repo satisfies the WrongRepo contract.
var _ WrongRepo = (*fakeWrongRepo)(nil)
