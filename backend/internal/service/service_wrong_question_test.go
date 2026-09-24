package service

import (
	"context"
	"log/slog"
	"sort"
	"testing"

	"github.com/gbexam/online-exam/internal/constants"
	"github.com/gbexam/online-exam/internal/dto"
	"github.com/gbexam/online-exam/internal/model"
	"github.com/gbexam/online-exam/internal/repository"
)

// fakeWrongRepo is an in-memory WrongRepo for service tests.
type fakeWrongRepo struct {
	records map[string]*model.WrongQuestion
	nextID  uint
}

func newFakeWrongRepo() *fakeWrongRepo {
	return &fakeWrongRepo{records: map[string]*model.WrongQuestion{}, nextID: 1}
}

func keyOf(studentID, questionID uint) string {
	return "s" + itoa(studentID) + "q" + itoa(questionID)
}

func itoa(v uint) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func (f *fakeWrongRepo) get(studentID, questionID uint) (*model.WrongQuestion, bool) {
	w, ok := f.records[keyOf(studentID, questionID)]
	return w, ok
}

func (f *fakeWrongRepo) UpsertWrongQuestion(_ context.Context, w *model.WrongQuestion) error {
	k := keyOf(w.StudentID, w.QuestionID)
	if existing, ok := f.records[k]; ok {
		if w.LastAttemptID == 0 || existing.LastAttemptID != w.LastAttemptID {
			existing.WrongCount++
		}
		existing.LostScore = w.LostScore
		existing.LastAttemptID = w.LastAttemptID
		existing.LastWrongAt = w.LastWrongAt
		existing.KnowledgePoint = w.KnowledgePoint
		existing.Status = constants.WrongUnresolved
		w.ID = existing.ID
		return nil
	}
	copy := *w
	copy.ID = f.nextID
	f.nextID++
	f.records[k] = &copy
	w.ID = copy.ID
	return nil
}

func (f *fakeWrongRepo) ListWrongQuestions(_ context.Context, studentID uint, _ string, _, _ int) ([]model.WrongQuestion, int64, error) {
	items := make([]model.WrongQuestion, 0)
	for _, w := range f.records {
		if w.StudentID == studentID {
			items = append(items, *w)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LastWrongAt.After(items[j].LastWrongAt) })
	return items, int64(len(items)), nil
}

func (f *fakeWrongRepo) DeleteWrongQuestion(_ context.Context, id, studentID uint) error {
	for k, w := range f.records {
		if w.ID == id && w.StudentID == studentID {
			delete(f.records, k)
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeWrongRepo) MarkWrongQuestionResolved(_ context.Context, id, studentID uint) error {
	for _, w := range f.records {
		if w.ID == id && w.StudentID == studentID {
			w.Status = constants.WrongResolved
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeWrongRepo) ResolveWrongQuestionByQuestion(_ context.Context, studentID, questionID uint) error {
	if w, ok := f.get(studentID, questionID); ok {
		w.Status = constants.WrongResolved
	}
	return nil
}

func (f *fakeWrongRepo) FindWrongQuestion(_ context.Context, studentID, questionID uint) (*model.WrongQuestion, error) {
	if w, ok := f.get(studentID, questionID); ok {
		copy := *w
		return &copy, nil
	}
	return nil, repository.ErrNotFound
}

// fakeQuestionRepo serves only the wrong-question service tests.
type fakeQuestionRepo struct {
	QuestionRepo
	questions map[uint]model.Question
}

func (f *fakeQuestionRepo) FindQuestionsByIDs(_ context.Context, ids []uint) (map[uint]model.Question, error) {
	out := map[uint]model.Question{}
	for _, id := range ids {
		if q, ok := f.questions[id]; ok {
			out[id] = q
		}
	}
	return out, nil
}

func (f *fakeQuestionRepo) FindQuestionByID(_ context.Context, id uint) (*model.Question, error) {
	if q, ok := f.questions[id]; ok {
		return &q, nil
	}
	return nil, repository.ErrNotFound
}

func newWrongService(wrong *fakeWrongRepo, questions ...model.Question) *WrongQuestionService {
	q := &fakeQuestionRepo{questions: map[uint]model.Question{}}
	for i := range questions {
		q.questions[questions[i].ID] = questions[i]
	}
	return NewWrongQuestionService(wrong, q, nil, nil, slog.Default())
}

func TestSubmitPracticeResolvesCorrectObjective(t *testing.T) {
	wrong := newFakeWrongRepo()
	svc := newWrongService(wrong, model.Question{
		ID: 1, Type: constants.QuestionSingle, Answer: `"B"`, Score: 2, KnowledgePoint: "数学",
	})
	wrong.UpsertWrongQuestion(context.Background(), &model.WrongQuestion{
		StudentID: 10, QuestionID: 1, WrongCount: 1, Status: constants.WrongUnresolved,
	})

	res, err := svc.SubmitPractice(context.Background(), 10, dto.PracticeAnswerRequest{
		Answers: []dto.PracticeAnswerItem{{QuestionID: 1, Answer: "B"}},
	})
	if err != nil {
		t.Fatalf("SubmitPractice: %v", err)
	}
	if res.Correct != 1 || res.Total != 1 || !res.Items[0].Correct {
		t.Fatalf("unexpected result: %+v", res)
	}
	rec, _ := wrong.get(10, 1)
	if rec.Status != constants.WrongResolved {
		t.Fatalf("expected resolved, got %q", rec.Status)
	}
}

func TestSubmitPracticeFillBlankWrongIncrements(t *testing.T) {
	wrong := newFakeWrongRepo()
	svc := newWrongService(wrong, model.Question{
		ID: 2, Type: constants.QuestionFillBlank, Answer: `["北京"]`, Score: 2, KnowledgePoint: "地理",
	})
	wrong.UpsertWrongQuestion(context.Background(), &model.WrongQuestion{
		StudentID: 10, QuestionID: 2, WrongCount: 1, Status: constants.WrongUnresolved,
	})

	res, err := svc.SubmitPractice(context.Background(), 10, dto.PracticeAnswerRequest{
		Answers: []dto.PracticeAnswerItem{{QuestionID: 2, Answer: []any{"上海"}}},
	})
	if err != nil {
		t.Fatalf("SubmitPractice: %v", err)
	}
	if res.Correct != 0 || !res.Items[0].AutoGraded || res.Items[0].Correct {
		t.Fatalf("unexpected result: %+v", res)
	}
	rec, _ := wrong.get(10, 2)
	if rec.Status != constants.WrongUnresolved || rec.WrongCount != 2 {
		t.Fatalf("expected unresolved count 2, got status=%q count=%d", rec.Status, rec.WrongCount)
	}
}

func TestSubmitPracticeShortAnswerDefersReview(t *testing.T) {
	wrong := newFakeWrongRepo()
	svc := newWrongService(wrong, model.Question{
		ID: 3, Type: constants.QuestionShortAnswer, Answer: `"200 表示成功"`, Analysis: "见 RFC", Score: 5, KnowledgePoint: "网络",
	})
	wrong.UpsertWrongQuestion(context.Background(), &model.WrongQuestion{
		StudentID: 10, QuestionID: 3, WrongCount: 1, Status: constants.WrongUnresolved,
	})

	res, err := svc.SubmitPractice(context.Background(), 10, dto.PracticeAnswerRequest{
		Answers: []dto.PracticeAnswerItem{{QuestionID: 3, Answer: "不知道"}},
	})
	if err != nil {
		t.Fatalf("SubmitPractice: %v", err)
	}
	if res.Total != 0 || len(res.Items) != 1 || res.Items[0].AutoGraded {
		t.Fatalf("short answer should not be auto graded: %+v", res)
	}
	if res.Items[0].CorrectAnswer != "200 表示成功" || res.Items[0].Analysis != "见 RFC" {
		t.Fatalf("missing reference answer: %+v", res.Items[0])
	}
	rec, _ := wrong.get(10, 3)
	if rec.WrongCount != 1 {
		t.Fatalf("submission alone must not change count, got %d", rec.WrongCount)
	}

	if err := svc.ReviewPractice(context.Background(), 10, dto.PracticeReviewRequest{QuestionID: 3, Correct: true}); err != nil {
		t.Fatalf("ReviewPractice: %v", err)
	}
	rec, _ = wrong.get(10, 3)
	if rec.Status != constants.WrongResolved {
		t.Fatalf("expected resolved after self review, got %q", rec.Status)
	}
}

func TestGradeSyncSubjectivePartialThenFull(t *testing.T) {
	wrong := newFakeWrongRepo()
	svc := &AttemptService{wrongRepo: wrong}
	q := &model.Question{ID: 7, KnowledgePoint: "语文", Score: 10}
	ctx := context.Background()

	// Teacher first awards 6/10: enters unmastered list with 4 lost points.
	if err := svc.syncSubjectiveWrongQuestion(ctx, 10, 100, q, 10, 6); err != nil {
		t.Fatalf("sync partial: %v", err)
	}
	rec, _ := wrong.get(10, 7)
	if rec.Status != constants.WrongUnresolved || rec.WrongCount != 1 || rec.LostScore != 4 || rec.LastAttemptID != 100 {
		t.Fatalf("unexpected record after partial grade: %+v", rec)
	}

	// Saving the same low score again for the same attempt must not double count.
	if err := svc.syncSubjectiveWrongQuestion(ctx, 10, 100, q, 10, 6); err != nil {
		t.Fatalf("sync repeated partial: %v", err)
	}
	rec, _ = wrong.get(10, 7)
	if rec.WrongCount != 1 {
		t.Fatalf("wrong count should stay 1, got %d", rec.WrongCount)
	}

	// A different exam losing points on the same question counts once more.
	if err := svc.syncSubjectiveWrongQuestion(ctx, 10, 101, q, 10, 8); err != nil {
		t.Fatalf("sync another attempt: %v", err)
	}
	rec, _ = wrong.get(10, 7)
	if rec.WrongCount != 2 || rec.LostScore != 2 {
		t.Fatalf("expected count 2 and lost 2, got count=%d lost=%v", rec.WrongCount, rec.LostScore)
	}

	// Teacher later awards full marks: mastered and leaves practice.
	if err := svc.syncSubjectiveWrongQuestion(ctx, 10, 101, q, 10, 10); err != nil {
		t.Fatalf("sync full: %v", err)
	}
	rec, _ = wrong.get(10, 7)
	if rec.Status != constants.WrongResolved {
		t.Fatalf("expected resolved after full score, got %q", rec.Status)
	}
}

func TestIsCorrectFillBlank(t *testing.T) {
	tests := []struct {
		name    string
		correct any
		student any
		want    bool
	}{
		{"match", []any{"北京"}, []any{"北京"}, true},
		{"trim and case", []any{"HTTP"}, []any{"  http "}, true},
		{"wrong blank", []any{"北京"}, []any{"上海"}, false},
		{"different blank count", []any{"a", "b"}, []any{"a"}, false},
		{"non slice", []any{"a"}, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCorrectFillBlank(tt.correct, tt.student); got != tt.want {
				t.Fatalf("isCorrectFillBlank(%v, %v) = %v, want %v", tt.correct, tt.student, got, tt.want)
			}
		})
	}
}
