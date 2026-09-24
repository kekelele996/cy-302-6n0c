<template>
  <div class="page-card">
    <div class="page-header">
      <h2>错题本</h2>
      <el-button type="primary" @click="startPractice">练习错题</el-button>
    </div>

    <el-table :data="rows" v-loading="loading" border>
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="question.content" label="题干" min-width="240" show-overflow-tooltip />
      <el-table-column prop="knowledge_point" label="知识点" width="130" />
      <el-table-column label="本场失分" width="110">
        <template #default="{ row }">
          <span :class="row.lost_score > 0 ? 'lost-score' : ''">{{ row.lost_score.toFixed(1) }} 分</span>
        </template>
      </el-table-column>
      <el-table-column prop="exam_title" label="所属考试" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ row.exam_title || '错题练习' }}</template>
      </el-table-column>
      <el-table-column prop="wrong_count" label="错误次数" width="90" />
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'resolved' ? 'success' : 'danger'">{{ row.status === 'resolved' ? '已掌握' : '未掌握' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" fixed="right">
        <template #default="{ row }">
          <el-button link type="danger" @click="onDelete(row)">移除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-pagination
      class="pager"
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="load"
    />

    <el-dialog v-model="practiceVisible" title="错题练习" width="760px" :close-on-click-modal="false">
      <template v-if="practice.length">
        <div v-for="(q, idx) in practice" :key="q.question_id" class="practice-item">
          <div class="answer-text">{{ idx + 1 }}. {{ q.content }} <el-tag size="small" type="info">{{ typeLabel(q.type) }}</el-tag></div>
          <div style="margin-top: 8px">
            <el-radio-group v-if="q.type === 'single' || q.type === 'true_false'" v-model="practiceAnswers[q.question_id]">
              <template v-if="q.type === 'single'">
                <el-radio v-for="opt in q.options" :key="opt.key" :value="opt.key">{{ opt.key }}. {{ opt.text }}</el-radio>
              </template>
              <template v-else>
                <el-radio value="T">正确</el-radio>
                <el-radio value="F">错误</el-radio>
              </template>
            </el-radio-group>
            <el-checkbox-group v-else-if="q.type === 'multiple'" v-model="practiceMultiple[q.question_id]">
              <el-checkbox v-for="opt in q.options" :key="opt.key" :value="opt.key">{{ opt.key }}. {{ opt.text }}</el-checkbox>
            </el-checkbox-group>
            <el-input
              v-else-if="q.type === 'fill_blank'"
              v-model="practiceTexts[q.question_id]"
              type="textarea"
              :rows="3"
              placeholder="每空一行填写答案"
            />
            <el-input
              v-else
              v-model="practiceTexts[q.question_id]"
              type="textarea"
              :rows="4"
              placeholder="请输入答案，提交后对照参考答案自行判断"
            />
          </div>
        </div>
      </template>
      <el-empty v-else description="没有需要练习的错题" />
      <template #footer>
        <el-button @click="practiceVisible = false">关闭</el-button>
        <el-button type="primary" :disabled="!practice.length" :loading="practicing" @click="submitPractice">提交</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="resultVisible" title="练习结果" width="760px">
      <div style="margin-bottom: 12px">自动判定：答对 {{ practiceResult?.correct ?? 0 }} / {{ practiceResult?.total ?? 0 }}</div>
      <div v-for="(item, idx) in practiceResult?.items || []" :key="item.question_id" class="result-item">
        <div class="result-title">
          {{ idx + 1 }}. {{ questionMap[item.question_id]?.content }}
          <el-tag v-if="item.auto_graded" :type="item.correct ? 'success' : 'danger'" size="small">
            {{ item.correct ? '回答正确' : '回答错误' }}
          </el-tag>
          <el-tag v-else type="warning" size="small">主观题，请自评</el-tag>
        </div>
        <template v-if="!item.auto_graded">
          <div class="reference">参考答案：{{ formatAnswer(item.correct_answer) }}</div>
          <div v-if="item.analysis" class="reference">解析：{{ item.analysis }}</div>
          <div class="review-actions">
            <el-button size="small" type="success" @click="review(item, true)">我答对了，标记已掌握</el-button>
            <el-button size="small" @click="review(item, false)">仍未掌握</el-button>
          </div>
        </template>
      </div>
      <template #footer>
        <el-button type="primary" @click="finishResult">完成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { wrongApi } from '../api'
import type { PracticeQuestion, PracticeResultResponse, WrongQuestionItem } from '../types'

const rows = ref<WrongQuestionItem[]>([])
const total = ref(0)
const loading = ref(false)
const practicing = ref(false)
const practiceVisible = ref(false)
const resultVisible = ref(false)
const practice = ref<PracticeQuestion[]>([])
const practiceAnswers = reactive<Record<number, unknown>>({})
const practiceMultiple = reactive<Record<number, string[]>>({})
const practiceTexts = reactive<Record<number, string>>({})
const practiceResult = ref<PracticeResultResponse | null>(null)
const query = reactive({ page: 1, page_size: 10 })

const questionMap = computed(() => {
  const map: Record<number, PracticeQuestion> = {}
  practice.value.forEach((q) => {
    map[q.question_id] = q
  })
  return map
})

async function load() {
  loading.value = true
  try {
    const res = await wrongApi.list({ ...query })
    rows.value = res.items
    total.value = res.total
  } finally {
    loading.value = false
  }
}

async function onDelete(row: WrongQuestionItem) {
  await ElMessageBox.confirm('确认移除该错题？', '提示', { type: 'warning' })
  await wrongApi.remove(row.id)
  ElMessage.success('已移除')
  load()
}

function typeLabel(type: string) {
  const map: Record<string, string> = {
    single: '单选题',
    multiple: '多选题',
    true_false: '判断题',
    fill_blank: '填空题',
    short_answer: '简答题'
  }
  return map[type] || type
}

function formatAnswer(answer: unknown): string {
  if (Array.isArray(answer)) return answer.join('；')
  return answer === undefined || answer === null ? '' : String(answer)
}

async function startPractice() {
  const res = await wrongApi.practice()
  practice.value = res.questions
  Object.keys(practiceAnswers).forEach((k) => delete practiceAnswers[Number(k)])
  Object.keys(practiceMultiple).forEach((k) => delete practiceMultiple[Number(k)])
  Object.keys(practiceTexts).forEach((k) => delete practiceTexts[Number(k)])
  practiceResult.value = null
  practiceVisible.value = true
}

async function submitPractice() {
  practicing.value = true
  try {
    const answers = practice.value.map((q) => {
      if (q.type === 'multiple') return { question_id: q.question_id, answer: practiceMultiple[q.question_id] || [] }
      if (q.type === 'fill_blank') return { question_id: q.question_id, answer: (practiceTexts[q.question_id] || '').split('\n') }
      if (q.type === 'short_answer') return { question_id: q.question_id, answer: practiceTexts[q.question_id] || '' }
      return { question_id: q.question_id, answer: practiceAnswers[q.question_id] || '' }
    })
    const res = await wrongApi.submitPractice(answers)
    practiceResult.value = res
    practiceVisible.value = false
    resultVisible.value = true
  } finally {
    practicing.value = false
  }
}

async function review(item: { question_id: number }, correct: boolean) {
  await wrongApi.reviewPractice(item.question_id, correct)
  ElMessage.success(correct ? '已标记为掌握' : '已记录，继续加油')
  load()
}

function finishResult() {
  resultVisible.value = false
  load()
}

onMounted(load)
</script>

<style scoped>
.pager {
  margin-top: 16px;
  justify-content: flex-end;
}
.practice-item {
  padding: 12px 0;
  border-bottom: 1px solid #f0f0f0;
}
.lost-score {
  color: var(--el-color-danger);
  font-weight: 600;
}
.result-item {
  padding: 10px 0;
  border-bottom: 1px solid #f0f0f0;
}
.result-title {
  margin-bottom: 6px;
}
.reference {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  margin-top: 4px;
}
.review-actions {
  margin-top: 8px;
}
</style>
