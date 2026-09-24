package repository

import (
	"context"
	"fmt"

	"github.com/gbexam/online-exam/internal/constants"
	"github.com/gbexam/online-exam/internal/model"
)

// UpsertWrongQuestion records a wrong answer.
//
// Repeated saves that refer to the same attempt (for example a teacher
// grading the same subjective answer several times) only refresh the lost
// score and status; the wrong count is not incremented again. When no
// attempt is associated (practice rounds) every call counts once.
func (r *Repository) UpsertWrongQuestion(ctx context.Context, w *model.WrongQuestion) error {
	var existing model.WrongQuestion
	err := r.db.WithContext(ctx).Where("student_id = ? AND question_id = ?", w.StudentID, w.QuestionID).First(&existing).Error
	if err == nil {
		updates := map[string]any{
			"lost_score":      w.LostScore,
			"last_wrong_at":   w.LastWrongAt,
			"knowledge_point": w.KnowledgePoint,
			"status":          constants.WrongUnresolved,
		}
		if w.LastAttemptID == 0 || existing.LastAttemptID != w.LastAttemptID {
			updates["wrong_count"] = existing.WrongCount + 1
		}
		updates["last_attempt_id"] = w.LastAttemptID
		res := r.db.WithContext(ctx).Model(&model.WrongQuestion{}).Where("id = ?", existing.ID).Updates(updates)
		if res.Error != nil {
			return fmt.Errorf("update wrong question: %w", res.Error)
		}
		w.ID = existing.ID
		return nil
	}
	if isRecordNotFound(err) {
		if createErr := r.db.WithContext(ctx).Create(w).Error; createErr != nil {
			return fmt.Errorf("create wrong question: %w", createErr)
		}
		return nil
	}
	return fmt.Errorf("find wrong question: %w", err)
}

// ListWrongQuestions returns a page of a student's wrong questions.
func (r *Repository) ListWrongQuestions(ctx context.Context, studentID uint, knowledgePoint string, page, pageSize int) ([]model.WrongQuestion, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.WrongQuestion{}).Where("student_id = ?", studentID)
	if knowledgePoint != "" {
		q = q.Where("knowledge_point = ?", knowledgePoint)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count wrong questions: %w", err)
	}
	var items []model.WrongQuestion
	p, ps := NormalizePage(page, pageSize)
	if err := q.Order("last_wrong_at DESC").Limit(ps).Offset((p - 1) * ps).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list wrong questions: %w", err)
	}
	return items, total, nil
}

// DeleteWrongQuestion removes one wrong question record.
func (r *Repository) DeleteWrongQuestion(ctx context.Context, id, studentID uint) error {
	res := r.db.WithContext(ctx).Where("id = ? AND student_id = ?", id, studentID).Delete(&model.WrongQuestion{})
	if res.Error != nil {
		return fmt.Errorf("delete wrong question: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkWrongQuestionResolved updates the status of one wrong question.
func (r *Repository) MarkWrongQuestionResolved(ctx context.Context, id, studentID uint) error {
	res := r.db.WithContext(ctx).Model(&model.WrongQuestion{}).Where("id = ? AND student_id = ?", id, studentID).Update("status", constants.WrongResolved)
	if res.Error != nil {
		return fmt.Errorf("resolve wrong question: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveWrongQuestionByQuestion marks a student's record for one question as
// mastered. It is a no-op when no record exists.
func (r *Repository) ResolveWrongQuestionByQuestion(ctx context.Context, studentID, questionID uint) error {
	res := r.db.WithContext(ctx).Model(&model.WrongQuestion{}).
		Where("student_id = ? AND question_id = ?", studentID, questionID).
		Update("status", constants.WrongResolved)
	if res.Error != nil {
		return fmt.Errorf("resolve wrong question by question: %w", res.Error)
	}
	return nil
}

// FindWrongQuestion returns one student's record for a question.
func (r *Repository) FindWrongQuestion(ctx context.Context, studentID, questionID uint) (*model.WrongQuestion, error) {
	var w model.WrongQuestion
	err := r.db.WithContext(ctx).Where("student_id = ? AND question_id = ?", studentID, questionID).First(&w).Error
	if err != nil {
		if isRecordNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find wrong question: %w", err)
	}
	return &w, nil
}
