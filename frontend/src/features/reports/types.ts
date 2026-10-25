export type ReportKind = 'week' | 'month';
export interface ReportField { title: string; require: string; sort: number; content: string }
export interface ReportDraft { start: string; end: string; content: ReportField[]; orphaned: ReportField[]; attachments: { name: string; uri: string }[]; pendingNames: string[]; submissionUncertain?: boolean }
export interface ReportAttachment { id: string; name: string; uri: string; file?: File }
export interface ReportFilters { start: string; end: string; status: string }
export type ReportRecord = Record<string, unknown>;
export interface ReportConfirmation { owner: string; version: number; key: string; epoch: number; input: { account: string; business_type: ReportKind; start_date: string; end_date: string; content: readonly Readonly<ReportField>[] }; attachments: readonly Readonly<ReportAttachment>[] }
