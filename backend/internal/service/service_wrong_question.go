package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/gbexam/online-exam/internal/constants"
	"github.com/gbexam/online-exam/internal/dto"
	"github.com/gbexam/online-exam/internal/model"
)

// WrongQuestionService manages the wrong-question book and practice.
type WrongQuestionService struct {
	baseService
	wrongRepo    WrongRepo
	questionRepo QuestionRepo
	attemptRepo  AttemptRepo
	examRepo     ExamRepo
}

// NewWrongQuestionService constructs WrongQuestionService.
func NewWrongQuestionService(wrongRepo WrongRepo, questionRepo QuestionRepo, attemptRepo AttemptRepo, examRepo ExamRepo, logger *slog.Logger) *WrongQuestionService {
	return &WrongQuestionService{
		baseService:  NewBaseService(logger),
		wrongRepo:    wrongRepo,
		questionRepo: questionRepo,
		attemptRepo:  attemptRepo,
		examRepo:     examRepo,
	}
}

// List returns a page of a student's wrong questions.
func (s *WrongQuestionService) List(ctx context.Context, studentID uint, query dto.WrongQuestionListQuery) (dto.PageResult, error) {
	records, total, err := s.wrongRepo.ListWrongQuestions(ctx, studentID, query.KnowledgePoint, query.Page, query.PageSize)
	if err != nil {
		return dto.PageResult{}, fmt.Errorf("list wrong questions: %w", err)
	}
	ids := make([]uint, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.QuestionID)
	}
	questionMap, err := s.questionRepo.FindQuestionsByIDs(ctx, ids)
	if err != nil {
		return dto.PageResult{}, fmt.Errorf("find questions by ids: %w", err)
	}
	examTitles := s.examTitlesFor(ctx, records)
	page, pageSize := normalizePage(query.Page, query.PageSize)
	items := make([]dto.WrongQuestionItem, 0, len(records))
	for _, r := range records {
		q, ok := questionMap[r.QuestionID]
		if !ok {
			continue
		}
		items = append(items, dto.WrongQuestionItem{
			ID:             r.ID,
			QuestionID:     r.QuestionID,
			KnowledgePoint: r.KnowledgePoint,
			WrongCount:     r.WrongCount,
			LostScore:      r.LostScore,
			ExamTitle:      examTitles[r.LastAttemptID],
			Status:         r.Status,
			LastWrongAt:    r.LastWrongAt,
			Question:       *questionToResponse(&q),
		})
	}
	return dto.PageResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// examTitlesFor resolves the exam title associated with each record's latest
// attempt, used to show which exam the lost points came from.
func (s *WrongQuestionService) examTitlesFor(ctx context.Context, records []model.WrongQuestion) map[uint]string {
	titles := make(map[uint]string, len(records))
	examCache := make(map[uint]string)
	for _, r := range records {
		if r.LastAttemptID == 0 {
			continue
		}
		if _, done := titles[r.LastAttemptID]; done {
			continue
		}
		attempt, err := s.attemptRepo.FindAttemptByID(ctx, r.LastAttemptID)
		if err != nil {
			titles[r.LastAttemptID] = ""
			continue
		}
		title, ok := examCache[attempt.ExamID]
		if !ok {
			if exam, err := s.examRepo.FindExamByID(ctx, attempt.ExamID); err == nil {
				title = exam.Title
			}
			examCache[attempt.ExamID] = title
		}
		titles[r.LastAttemptID] = title
	}
	return titles
}

// Delete removes a wrong-question record.
func (s *WrongQuestionService) Delete(ctx context.Context, studentID, id uint) error {
	return s.wrongRepo.DeleteWrongQuestion(ctx, id, studentID)
}

// Practice returns a shuffled set of unresolved wrong questions.
func (s *WrongQuestionService) Practice(ctx context.Context, studentID uint) (*dto.PracticeStartResponse, error) {
	records, _, err := s.wrongRepo.ListWrongQuestions(ctx, studentID, "", 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("list wrong questions: %w", err)
	}
	unresolved := make([]model.WrongQuestion, 0, len(records))
	for _, r := range records {
		if r.Status == constants.WrongUnresolved {
			unresolved = append(unresolved, r)
		}
	}
	ids := make([]uint, 0, len(unresolved))
	for _, r := range unresolved {
		ids = append(ids, r.QuestionID)
	}
	questionMap, err := s.questionRepo.FindQuestionsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("find questions by ids: %w", err)
	}
	questions := make([]model.Question, 0, len(ids))
	for _, id := range ids {
		if q, ok := questionMap[id]; ok {
			questions = append(questions, q)
		}
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffle(questions, rng)

	items := make([]dto.PracticeQuestion, 0, len(questions))
	for i := range questions {
		q := questions[i]
		options, _ := unmarshalOptions(q.Options)
		if isChoiceType(q.Type) {
			shuffle(options, rng)
		}
		items = append(items, dto.PracticeQuestion{
			QuestionID:     q.ID,
			Type:           q.Type,
			Content:        q.Content,
			Options:        options,
			Score:          q.Score,
			KnowledgePoint: q.KnowledgePoint,
		})
	}
	return &dto.PracticeStartResponse{Questions: items}, nil
}

// SubmitPractice grades a practice set and updates wrong-question status.
// Objective and fill-in-the-blank questions are graded automatically; short
// answer questions return the reference answer for student self-assessment
// via ReviewPractice.
func (s *WrongQuestionService) SubmitPractice(ctx context.Context, studentID uint, req dto.PracticeAnswerRequest) (*dto.PracticeResultResponse, error) {
	ids := make([]uint, 0, len(req.Answers))
	for _, a := range req.Answers {
		ids = append(ids, a.QuestionID)
	}
	questionMap, err := s.questionRepo.FindQuestionsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("find questions by ids: %w", err)
	}
	records, _, err := s.wrongRepo.ListWrongQuestions(ctx, studentID, "", 1, 1000)
	if err != nil {
		return nil, fmt.Errorf("list wrong questions: %w", err)
	}
	recordByQuestion := make(map[uint]model.WrongQuestion, len(records))
	for _, r := range records {
		recordByQuestion[r.QuestionID] = r
	}

	result := &dto.PracticeResultResponse{Items: make([]dto.PracticeResultItem, 0, len(req.Answers))}
	for _, a := range req.Answers {
		q, ok := questionMap[a.QuestionID]
		if !ok {
			continue
		}
		correctAnswer, _ := unmarshalAnswer(q.Answer)
		item := dto.PracticeResultItem{QuestionID: q.ID}
		if q.Type != constants.QuestionShortAnswer {
			// Choice questions and fill-in-the-blank are auto-gradable.
			correct := false
			if ObjectiveQuestionTypes()[q.Type] {
				correct = isCorrectObjective(q.Type, correctAnswer, a.Answer)
			} else {
				correct = isCorrectFillBlank(correctAnswer, a.Answer)
			}
			item.AutoGraded = true
			item.Correct = correct
			if correct {
				item.Score = q.Score
				if record, exists := recordByQuestion[q.ID]; exists {
					if err := s.wrongRepo.MarkWrongQuestionResolved(ctx, record.ID, studentID); err != nil {
						return nil, fmt.Errorf("resolve wrong question: %w", err)
					}
				}
			} else if err := s.recordPracticeWrong(ctx, studentID, &q); err != nil {
				return nil, err
			}
			result.Items = append(result.Items, item)
			result.Total++
			if item.Correct {
				result.Correct++
			}
			continue
		}
		// Short answer: defer to student self-assessment.
		item.AutoGraded = false
		item.CorrectAnswer = correctAnswer
		item.Analysis = q.Analysis
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// ReviewPractice records a student's self-assessment of a subjective practice
// question: a correct self-review marks it mastered, otherwise it stays in
// the unmastered list.
func (s *WrongQuestionService) ReviewPractice(ctx context.Context, studentID uint, req dto.PracticeReviewRequest) error {
	q, err := s.questionRepo.FindQuestionByID(ctx, req.QuestionID)
	if err != nil {
		return err
	}
	record, err := s.wrongRepo.FindWrongQuestion(ctx, studentID, req.QuestionID)
	if err != nil {
		return ErrNotFound
	}
	if req.Correct {
		if err := s.wrongRepo.MarkWrongQuestionResolved(ctx, record.ID, studentID); err != nil {
			return fmt.Errorf("resolve wrong question: %w", err)
		}
		return nil
	}
	return s.recordPracticeWrong(ctx, studentID, q)
}

func (s *WrongQuestionService) recordPracticeWrong(ctx context.Context, studentID uint, q *model.Question) error {
	w := &model.WrongQuestion{
		StudentID:      studentID,
		QuestionID:     q.ID,
		KnowledgePoint: q.KnowledgePoint,
		WrongCount:     1,
		LostScore:      q.Score,
		LastWrongAt:    time.Now(),
		Status:         constants.WrongUnresolved,
	}
	if err := s.wrongRepo.UpsertWrongQuestion(ctx, w); err != nil {
		return fmt.Errorf("upsert wrong question: %w", err)
	}
	return nil
}

// isCorrectFillBlank compares blank answers in order, ignoring surrounding
// whitespace and letter case.
func isCorrectFillBlank(correct, student any) bool {
	c, ok1 := toStringSlice(correct)
	st, ok2 := toStringSlice(student)
	if !ok1 || !ok2 || len(c) == 0 || len(c) != len(st) {
		return false
	}
	for i := range c {
		if !strings.EqualFold(strings.TrimSpace(c[i]), strings.TrimSpace(st[i])) {
			return false
		}
	}
	return true
}
