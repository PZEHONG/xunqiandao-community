import { useEffect, useRef, useState } from 'react';
import { Box, Button, Stack, TextField, Typography } from '@mui/material';
import type { School } from '../../api/contracts';
import { searchSchools } from './api';
export function SchoolAutocomplete({ value, onSelect, disabled }: { value: School | null; onSelect: (school: School | null) => void; disabled: boolean }) {
  const [query, setQuery] = useState(value?.school_name ?? ''); const [composing, setComposing] = useState(false);
  const [results, setResults] = useState<School[]>([]); const [hint, setHint] = useState('输入至少 2 个字，并从匹配结果中选择学校。'); const sequence = useRef(0);
  useEffect(() => {
    const seq = ++sequence.current; const controller = new AbortController();
    if (composing || value || [...query.trim()].length < 2 || disabled) return () => controller.abort();
    const timer = setTimeout(() => {
      setHint('正在匹配学校…');
      void searchSchools(query.trim(), controller.signal).then(schools => { if (seq !== sequence.current || controller.signal.aborted) return; setResults(schools); setHint(schools.length ? '请选择匹配的学校。' : '未找到学校，请修改名称。'); }).catch(() => { if (seq === sequence.current && !controller.signal.aborted) setHint('学校匹配失败，请修改名称后重试。'); });
    }, 350);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [query, composing, value, disabled]);
  const update = (text: string) => { sequence.current++; setQuery(text); setResults([]); onSelect(null); setHint('输入至少 2 个字，并从匹配结果中选择学校。'); };
  return <Stack gap={1}><TextField label="学校名称" autoComplete="organization" value={query} disabled={disabled} onChange={event => update(event.target.value)} onCompositionStart={() => { setComposing(true); setResults([]); sequence.current++; }} onCompositionEnd={() => setComposing(false)} helperText={value ? `已选择：${value.school_name}` : hint} inputProps={{ role: 'combobox', 'aria-autocomplete': 'list', 'aria-expanded': results.length > 0, 'aria-controls': results.length ? 'school-options' : undefined }} />
    {results.length > 0 && <Box id="school-options" role="listbox" aria-label="匹配学校" sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, maxHeight: 180, overflowY: 'auto' }}>{results.map(school => <Button key={school.school_id} role="option" aria-selected={false} fullWidth sx={{ justifyContent: 'flex-start', textAlign: 'left' }} onClick={() => { sequence.current++; onSelect(school); setQuery(school.school_name); setResults([]); }}>{school.school_name}</Button>)}</Box>}
    {disabled && <Typography variant="caption">请稍候…</Typography>}
  </Stack>;
}
