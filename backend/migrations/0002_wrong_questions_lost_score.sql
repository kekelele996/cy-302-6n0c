-- 错题本增加“本场失分”列，记录最近一次考试该题的失分值。
ALTER TABLE wrong_questions
    ADD COLUMN lost_score DOUBLE NOT NULL DEFAULT 0 AFTER wrong_count;
