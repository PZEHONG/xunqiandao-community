import type { ConfirmedLocation } from '../locations/types';
export interface SignRemark { key: string; value: string }
export interface SignDraft { readonly remark: string; readonly comment: string; readonly photos: readonly File[] }
export interface SignConfirmation {
  readonly owner: string; readonly epoch: number; readonly account: string; readonly school: string; readonly version: number; readonly locationVersion: number;
  readonly location: ConfirmedLocation; readonly remark: string; readonly remarkLabel: string; readonly comment: string; readonly photos: readonly File[];
}
export interface SignResult { status: 'success' | 'unknown'; message: string }
export interface SignLocation { confirmed: ConfirmedLocation | null; pending: boolean }
export function photoError(photos: readonly File[]): string {
  if (photos.length > 3) return '最多添加3张照片。';
  if (photos.some(file => !['image/jpeg', 'image/png', 'image/gif', 'image/webp'].includes(file.type) || !file.size || file.size > 10 * 1024 * 1024)) return '请选择10 MB以内的JPG、PNG、GIF或WebP图片，照片不能为空。';
  return '';
}
