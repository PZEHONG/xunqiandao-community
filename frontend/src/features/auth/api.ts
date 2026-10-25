import { request } from '../../api/client';
import type { School } from '../../api/contracts';
export async function searchSchools(name: string, signal: AbortSignal) {
  const result = await request<{ schools: School[] }>(`/api/auth/schools?school_name=${encodeURIComponent(name)}`, { signal });
  return (Array.isArray(result.schools) ? result.schools : []).filter(school => school && school.school_id != null && String(school.school_id).trim() && typeof school.school_name === 'string' && school.school_name.trim()).map(school => ({ ...school, school_id: String(school.school_id) }));
}
