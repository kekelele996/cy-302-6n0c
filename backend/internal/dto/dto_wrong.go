package dto

import "time"

// WrongQuestionListQuery filters the wrong-question book.
type WrongQuestionListQuery struct {
	PageQuery
	KnowledgePoint string `form:"knowledge_point"`
}

// WrongQuestionItem is one wrong-question record with its question.
type WrongQuestionItem struct {
	ID             uint             `json:"id"`
	QuestionID     uint             `json:"question_id"`
	KnowledgePoint string           `json:"knowledge_point"`
	WrongCount     int              `json:"wrong_count"`
	LostScore      float64          `json:"lost_score"`
	ExamTitle      string           `json:"exam_title"`
	Status         string           `json:"status"`
	LastWrongAt    time.Time        `json:"last_wrong_at"`
	Question       QuestionResponse `json:"question"`
}

// PracticeQuestion is a question without an answer in a practice set.
type PracticeQuestion struct {
	QuestionID     uint     `json:"question_id"`
	Type           string   `json:"type"`
	Content        string   `json:"content"`
	Options        []Option `json:"options"`
	Score          float64  `json:"score"`
	KnowledgePoint string   `json:"knowledge_point"`
}

// PracticeStartResponse returns a shuffled practice set.
type PracticeStartResponse struct {
	Questions []PracticeQuestion `json:"questions"`
}

// PracticeAnswerItem is one answer in practice submission.
type PracticeAnswerItem struct {
	QuestionID uint `json:"question_id" binding:"required"`
	Answer     any  `json:"answer"`
}

// PracticeAnswerRequest submits a practice set.
type PracticeAnswerRequest struct {
	Answers []PracticeAnswerItem `json:"answers" binding:"required,min=1,dive"`
}

// PracticeResultItem reports one practice answer. Subjective questions that
// cannot be graded automatically return the reference answer for review.
type PracticeResultItem struct {
	QuestionID    uint    `json:"question_id"`
	Correct       bool    `json:"correct"`
	AutoGraded    bool    `json:"auto_graded"`
	Score         float64 `json:"score"`
	CorrectAnswer any     `json:"correct_answer,omitempty"`
	Analysis      string  `json:"analysis,omitempty"`
}

// PracticeResultResponse summarizes a practice round.
type PracticeResultResponse struct {
	Total   int                  `json:"total"`
	Correct int                  `json:"correct"`
	Items   []PracticeResultItem `json:"items"`
}

// PracticeReviewRequest lets a student self-assess a subjective practice
// question after comparing it with the reference answer.
type PracticeReviewRequest struct {
	QuestionID uint `json:"question_id" binding:"required"`
	Correct    bool `json:"correct"`
}
