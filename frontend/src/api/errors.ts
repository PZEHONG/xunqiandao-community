export class ApiError extends Error {
  constructor(public status: number, message: string, public code?: string) { super(message); this.name = 'ApiError'; }
}
export class UnknownSubmissionError extends ApiError {
  constructor(sign = false) { super(0, sign ? '签到提交结果暂时无法确认，请先刷新签到记录，再决定是否重新提交。' : '提交结果暂时无法确认，请先查询结果，再决定是否重新提交。', 'UNKNOWN_SUBMISSION'); this.name = 'UnknownSubmissionError'; }
}
export class StaleSessionError extends Error { constructor() { super('账号状态已变化，已忽略旧响应。'); this.name = 'StaleSessionError'; } }
