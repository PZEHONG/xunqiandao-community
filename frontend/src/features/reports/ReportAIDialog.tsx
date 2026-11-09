import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Checkbox, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, LinearProgress, Stack, TextField, Typography } from '@mui/material';
import AutoFixHighOutlined from '@mui/icons-material/AutoFixHighOutlined';
import StopCircleOutlined from '@mui/icons-material/StopCircleOutlined';
import { useSession } from '../../session/useSession';
import { getSessionEpoch } from '../../session/sessionEpoch';
import { aiDraftKey, generateAIReport, loadAIDrafts, missingReports, saveAIDrafts, suggestReportDates, type AIDraft, type AIPeriod } from './reportAI';
import { downloadText, reportLabel, validRange } from './reportKinds';
import type { ReportsModel } from './useReports';
import type { ReportKind } from './types';
import './reportAIDialog.css';

export function ReportAIDialog({ model: m, kind, mode, onClose }: { model: ReportsModel; kind: ReportKind; mode: 'single' | 'batch'; onClose: () => void }) {
 const session = useSession(); const epoch = session.epoch; const storageKey = aiDraftKey(m.key);
 // List filters use submission times; completion must start from actual report dates.
 const [start, setStart] = useState(mode === 'single' ? m.start : ''); const [end, setEnd] = useState(mode === 'single' ? m.end : '');
 const [selectedPeriods, setSelectedPeriods] = useState<Set<string>>(() => new Set());
 const [notes, setNotes] = useState(''); const [periods, setPeriods] = useState<AIPeriod[] | null>(null);
 const [drafts, setDrafts] = useState<AIDraft[]>(() => loadAIDrafts(storageKey)); const draftRef = useRef(drafts);
 const [showSetup, setShowSetup] = useState(drafts.length === 0);
 const [busy, setBusy] = useState(false); const [checking, setChecking] = useState(false); const [error, setError] = useState(''); const [message, setMessage] = useState(''); const [storageError, setStorageError] = useState(false);
 const [selected, setSelected] = useState<number | null>(drafts.length ? 0 : null); const [importing, setImporting] = useState(false); const [importConfirm, setImportConfirm] = useState(false);
 const [progress, setProgress] = useState({ current: 0, total: 0 }); const job = useRef<AbortController | null>(null); const alive = useRef(true); const running = useRef(false);
 const datesEdited = useRef(!!(start || end)); const dateJob = useRef<AbortController | null>(null);
 const datePending = useRef(false);
 const [suggesting, setSuggesting] = useState(false); const [dateError, setDateError] = useState('');
 const current = () => alive.current && getSessionEpoch() === epoch;
 const fillDates = useCallback(async (replace = false) => {
  if (running.current) return;
  dateJob.current?.abort(); const controller = new AbortController(); dateJob.current = controller;
  datePending.current = true;
  setSuggesting(true); setDateError('');
  try {
   const result = await suggestReportDates(m.account, kind, mode, epoch, controller.signal);
   if (!alive.current || controller.signal.aborted || getSessionEpoch() !== epoch) return;
   if (!datesEdited.current || replace) { setStart(result.start); setEnd(result.end); setPeriods(null); setSelectedPeriods(new Set()); setDateError(result.start && result.end ? '' : result.reason); }
  } catch { if (alive.current && !controller.signal.aborted && getSessionEpoch() === epoch) setDateError('暂时无法读取历史报告，请手动选择日期。'); }
  finally { if (dateJob.current === controller) { datePending.current = false; if (alive.current) setSuggesting(false); } }
 }, [m.account, kind, mode, epoch]);
 useEffect(() => {
  alive.current = true;
  if (!datesEdited.current) void fillDates();
  return () => { alive.current = false; dateJob.current?.abort(); job.current?.abort(); };
 }, [fillDates]);
 const keepDrafts = (next: AIDraft[]) => { if (!current()) return; draftRef.current = next; setDrafts(next); setStorageError(!saveAIDrafts(storageKey, next)); };
 const setDates = (a: string, b: string) => { datesEdited.current = true; datePending.current = false; dateJob.current?.abort(); setSuggesting(false); setDateError(''); setStart(a); setEnd(b); setPeriods(null); setSelectedPeriods(new Set()); setMessage(''); setError(''); };
 const check = async () => {
  if (datePending.current) return;
  if (running.current || !validRange(start, end)) { setError('请先选择有效的起止日期。'); return; }
  const controller = new AbortController(); job.current = controller; running.current = true; setChecking(true); setError(''); setPeriods(null); setSelectedPeriods(new Set());
  try { const result = await missingReports(m.account, kind, start, end, epoch, controller.signal); if (current()) setPeriods(result); }
  catch (e) { if (current() && !controller.signal.aborted) setError(e instanceof Error ? e.message : '缺报查询失败。'); }
  finally { if (current()) { running.current = false; setChecking(false); } }
 };
 const periodKey = (p: { start: string; end: string }) => `${p.start}:${p.end}`;
 const hasDraft = (p: AIPeriod) => drafts.some(d => d.start === p.start && d.end === p.end && d.status === 'ready');
 const selectable = (periods || []).filter(p => p.state === 'missing' && !hasDraft(p));
 const selectedCount = selectable.filter(p => selectedPeriods.has(periodKey(p))).length;
 const selectionDisabled = busy || checking || suggesting || importing;
 const togglePeriod = (period: AIPeriod, checked: boolean) => setSelectedPeriods(previous => {
  const next = new Set(previous); if (checked) next.add(periodKey(period)); else next.delete(periodKey(period)); return next;
 });
 const targets = mode === 'single' ? [{ start, end, state: 'missing' as const }] : selectable.filter(p => selectedPeriods.has(periodKey(p)));
 const pending = targets.filter(p => !drafts.some(d => d.start === p.start && d.end === p.end && d.status === 'ready'));
 const generate = async (retry?: AIDraft) => {
  if (running.current || datePending.current) return;
  if (!validRange(retry?.start ?? start, retry?.end ?? end) || !notes.trim()) { setShowSetup(true); setError('请选择报告日期，并填写岗位、工作内容或学习记录。'); return; }
  const queue = retry ? [{ start: retry.start, end: retry.end }] : pending;
  if (!queue.length) return;
  running.current = true; const controller = new AbortController(); job.current = controller;
  setBusy(true); setShowSetup(false); setError(''); setMessage(''); setProgress({ current: 0, total: queue.length });
  try {
   for (let n = 0; n < queue.length; n++) {
    if (controller.signal.aborted || !current()) break;
    const period = queue[n]; let active: AIDraft = { start: period.start, end: period.end, fields: [], status: 'generating' };
    // Starting a retry replaces only this period, before any response arrives.
    let index = draftRef.current.findIndex(d => d.start === period.start && d.end === period.end);
    if (index < 0) { index = draftRef.current.length; keepDrafts([...draftRef.current, active]); }
    else keepDrafts(draftRef.current.map((d, i) => i === index ? active : d));
    setSelected(index); setProgress({ current: n + 1, total: queue.length });
    const replace = () => keepDrafts(draftRef.current.map((d, i) => i === index ? { ...active, fields: active.fields.map(f => ({ ...f })) } : d));
    try {
     await generateAIReport({ account: m.account, business_type: kind, start: period.start, end: period.end, notes }, epoch, controller.signal, event => {
      if (!current()) return;
      if (event.type === 'field') { active.fields[event.index] = { ...event.field, content: '' }; replace(); }
      if (event.type === 'delta') { if (!active.fields[event.index]) throw new Error('AI 栏目数据不完整。'); active.fields[event.index].content += event.delta; replace(); }
      if (event.type === 'done') { active = { ...active, fields: event.fields, status: 'ready' }; replace(); }
     });
    } catch (e) {
     active.status = 'partial'; replace();
     throw e;
    }
   }
   if (current()) {
    if (controller.signal.aborted) setError('已停止生成，已保留生成内容。');
    else setMessage('草稿已生成，请逐份核对、补充真实细节后再提交。');
   }
  } catch (e) { if (current()) { setError(controller.signal.aborted ? '已停止生成，已保留生成内容。' : e instanceof Error ? e.message : 'AI 生成失败，已保留生成内容。'); } }
  finally { if (current()) { running.current = false; setBusy(false); keepDrafts(draftRef.current.map(d => d.status === 'generating' ? { ...d, status: 'partial' } : d)); } }
 };
 const chosen = selected === null ? null : drafts[selected];
 const apply = async () => {
  if (!chosen || importing) return; setImporting(true); setError('');
  try {
   await m.importAIDraft(chosen);
   if (current()) {
    const remaining = draftRef.current.filter(d => d.start !== chosen.start || d.end !== chosen.end);
    if (!saveAIDrafts(storageKey, remaining)) throw new Error('正文已载入编辑器，但 AI 草稿未能清除，请稍后重试。');
    draftRef.current = remaining; setDrafts(remaining); setSelected(remaining.length ? 0 : null); onClose();
   }
  }
  catch (e) { if (current()) setError(e instanceof Error ? e.message : '载入编辑器失败。'); }
  finally { if (current()) { setImporting(false); setImportConfirm(false); } }
 };
 return <><Dialog open fullWidth maxWidth="md" onClose={() => { if (!busy && !checking && !importing) onClose(); }} aria-labelledby="ai-report-title" scroll="paper" slotProps={{ paper: { className: `report-ai-dialog${busy ? ' is-generating' : ''}` } }}>
  <DialogTitle id="ai-report-title">{mode === 'single' ? 'AI 起草' : 'AI补全'} · {reportLabel(kind)}</DialogTitle>
  <DialogContent dividers><Stack gap={2.5}>
   <Typography variant="body2" color="text.secondary">根据填写日期和工作素材生成草稿，最终由你核对并提交。</Typography>
   <Button sx={{ alignSelf: 'flex-start' }} disabled={busy || checking || importing} onClick={() => setShowSetup(!showSetup)}>{showSetup ? '收起写作设置' : '调整日期与素材'}</Button>
   {showSetup && <Stack gap={2.5}>
   <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}><TextField fullWidth label="报告起始日期" type="date" value={start} disabled={busy || checking} onChange={e => setDates(e.target.value, end)} slotProps={{ inputLabel: { shrink: true } }} /><TextField fullWidth label="报告截止日期" type="date" value={end} disabled={busy || checking} onChange={e => setDates(start, e.target.value)} slotProps={{ inputLabel: { shrink: true } }} /></Stack>
   <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
    <Button variant="outlined" disabled={busy || checking || suggesting || importing} onClick={() => void fillDates(true)} endIcon={<AutoFixHighOutlined className="date-fill-reveal" data-testid="date-fill-reveal" aria-hidden="true" />} sx={{
     minHeight: 44,
     '& .date-fill-reveal': { opacity: 0, transform: 'translateX(-3px)', transition: 'opacity 140ms ease, transform 140ms ease' },
     '&:hover .date-fill-reveal, &:focus-visible .date-fill-reveal, &.Mui-focusVisible .date-fill-reveal': { opacity: 1, transform: 'translateX(0)' },
     '@media (hover: none)': { '& .date-fill-reveal': { opacity: 1, transform: 'none' } },
     '@media (prefers-reduced-motion: reduce)': { '& .date-fill-reveal': { transition: 'none', transform: 'none' } },
    }}>{suggesting ? '智能填写中…' : '智能填写日期'}</Button>
    {mode === 'batch' && <Button variant="outlined" sx={{ minHeight: 44 }} disabled={busy || checking || suggesting || importing || !validRange(start, end)} onClick={() => void check()}>{checking ? '正在核对历史报告…' : '查询缺报'}</Button>}
   </Stack>
   {dateError && <Alert severity="warning">{dateError}</Alert>}
   {mode === 'batch' && periods && <Stack gap={1}>
    <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
     <FormControlLabel sx={{ m: 0 }} label="全选" control={<Checkbox checked={selectable.length > 0 && selectedCount === selectable.length} indeterminate={selectedCount > 0 && selectedCount < selectable.length} disabled={selectionDisabled || !selectable.length} onChange={e => setSelectedPeriods(e.target.checked ? new Set(selectable.map(periodKey)) : new Set())} slotProps={{ input: { 'aria-label': '全选缺报', 'aria-checked': selectedCount > 0 && selectedCount < selectable.length ? 'mixed' : selectable.length > 0 && selectedCount === selectable.length } }} />} />
     <Typography variant="body2" color="text.secondary" role="status">已选 {selectedCount} 段</Typography>
     <Chip label={`待补全 ${periods.filter(p => p.state === 'missing').length} 段`} color="primary" variant="outlined" />
     <Chip label={`已覆盖 ${periods.filter(p => p.state === 'existing').length} 段`} variant="outlined" />
     {periods.some(p => p.state === 'overlap') && <Chip label={`重叠 ${periods.filter(p => p.state === 'overlap').length} 段`} variant="outlined" />}
    </Stack>
    <Stack gap={.5} sx={{ maxHeight: 264, overflowY: 'auto' }}>
     {periods.map(p => {
      const eligible = p.state === 'missing' && !hasDraft(p); const checked = eligible && selectedPeriods.has(periodKey(p)); const label = `${p.start} 至 ${p.end}`;
      return <Stack key={periodKey(p)} direction="row" alignItems="center" gap={1} sx={{ pr: 1, borderRadius: 1, bgcolor: checked ? 'action.selected' : 'transparent' }}>
       <FormControlLabel sx={{ m: 0, flex: 1, minWidth: 0 }} label={<Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{label}</Typography>} control={<Checkbox checked={checked} disabled={selectionDisabled || !eligible} onChange={e => togglePeriod(p, e.target.checked)} slotProps={{ input: { 'aria-label': `补全 ${label}` } }} />} />
       <Typography variant="body2" sx={{ flexShrink: 0 }} color={eligible ? 'primary.main' : 'text.secondary'}>{hasDraft(p) ? '已有草稿' : ({ missing: '待补全', existing: '已覆盖', overlap: '重叠，需核对' })[p.state]}</Typography>
      </Stack>;
     })}
    </Stack>
   </Stack>}
   <TextField label="工作素材" multiline minRows={3} value={notes} disabled={busy} onChange={e => setNotes(e.target.value)} helperText="填写岗位、具体任务、学习收获；批量生成时可按日期分段说明。信息不足会标注待补充。" slotProps={{ htmlInput: { maxLength: 6000 } }} />
   </Stack>}
   {error && <Alert severity="error">{error}</Alert>}{message && <Alert severity="success">{message}</Alert>}{storageError && <Alert severity="warning">浏览器未能保存 AI 草稿，请下载备份后再关闭。</Alert>}
   <Stack direction="row" gap={1} flexWrap="wrap"><Button variant="contained" disabled={busy || checking || suggesting || importing || !pending.length || (mode === 'batch' && !periods)} onClick={() => void generate()}>{mode === 'single' ? (pending.length ? '生成草稿' : '该日期已有草稿') : `生成所选草稿${pending.length ? `（${pending.length}）` : ''}`}</Button></Stack>
   {busy && <Box role="status"><Typography variant="body2" sx={{ mb: 1 }}>正在生成第 {progress.current} / {progress.total} 份，正文将逐步显示…</Typography><LinearProgress /></Box>}
   {!!drafts.length && <Stack gap={1.5}><Typography component="h3" variant="h3">AI 草稿 · {drafts.length} 份</Typography><Stack direction="row" gap={1} flexWrap="wrap">{drafts.map((d, i) => <Button key={`${d.start}:${d.end}`} variant={selected === i ? 'contained' : 'outlined'} size="small" onClick={() => setSelected(i)}>{d.start} · {d.status === 'ready' ? '待核对' : d.status === 'generating' ? '生成中' : '未完成'}</Button>)}</Stack>
    {chosen && <Box sx={{ p: { xs: 1.5, sm: 2 }, border: 1, borderColor: 'divider', borderRadius: 2 }}><Stack gap={2}><Typography fontWeight={600}>{chosen.start} 至 {chosen.end}</Typography>{chosen.fields.map((f, i) => <Box key={`${i}:${f.sort}`}><Typography fontWeight={600} variant="body2" sx={{ mb: .75 }}>{f.title}</Typography><Typography sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', lineHeight: 1.8 }}>{f.content || (chosen.status === 'generating' ? '等待生成…' : '尚未生成正文，可重新生成此份。')}</Typography></Box>)}<Stack direction="row" gap={1} flexWrap="wrap"><Button variant="outlined" disabled={busy || checking || importing || !chosen.fields.some(f => f.content)} onClick={() => setImportConfirm(true)}>载入编辑器核对</Button><Button disabled={busy} onClick={() => downloadText(`${reportLabel(kind)}-${chosen.start}.txt`, [chosen.start + ' 至 ' + chosen.end, ...chosen.fields.map(f => `${f.title}\n${f.content}`)].join('\n\n'))}>下载草稿</Button><Button disabled={busy || checking || suggesting || importing} onClick={() => void generate(chosen)}>重新生成此份</Button></Stack></Stack></Box>}
   </Stack>}
  </Stack></DialogContent><DialogActions>
   {(busy || checking) && <Button variant="outlined" color="error" startIcon={<StopCircleOutlined />} sx={{ minHeight: 44 }} onClick={() => job.current?.abort()}>{busy ? '停止生成' : '停止查询'}</Button>}
   <Button disabled={busy || checking || importing} onClick={onClose}>关闭</Button>
  </DialogActions>
 </Dialog><Dialog open={importConfirm} onClose={() => { if (!importing) setImportConfirm(false); }} aria-labelledby="ai-import-title"><DialogTitle id="ai-import-title">载入 AI 草稿</DialogTitle><DialogContent><Typography>将替换当前编辑草稿的日期和正文，保留已有附件。成功载入并保存后会清除这份 AI 草稿，其他 AI 草稿保留。你可继续修改，再确认提交。</Typography></DialogContent><DialogActions><Button disabled={importing} onClick={() => setImportConfirm(false)}>取消</Button><Button variant="contained" disabled={importing} onClick={() => void apply()}>{importing ? '载入中…' : '确认载入'}</Button></DialogActions></Dialog></>;
}
