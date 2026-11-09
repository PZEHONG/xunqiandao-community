import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { RunHistory } from './RunHistory';
it.each(['failed','unknown','skipped','unexpected'])('shows unsuccessful terminal status %s as failure', status => {
 render(<RunHistory runs={[{id:1,task_id:1,account:'u',name:'每日签到',revision:1,date:'2026-10-23',scheduled_time:'20:15',status,message:''}]} />);
 expect(screen.getByText('失败')).toBeInTheDocument();
 expect(screen.queryByText('待核对')).not.toBeInTheDocument();
});
it('shows an existing official sign as signed with its explanation', () => {
 render(<RunHistory runs={[{id:2,task_id:2,account:'u',name:'每日签到',revision:1,date:'2026-10-26',scheduled_time:'12:30',status:'already_signed',message:'今日已有签到记录，本次未重复签到'}]} />);
 expect(screen.getByText('已签到')).toBeInTheDocument();
 expect(screen.getByText('今日已有签到记录，本次未重复签到')).toBeInTheDocument();
 expect(screen.queryByText('失败')).not.toBeInTheDocument();
 expect(screen.queryByText('成功')).not.toBeInTheDocument();
});
it('shows an ambiguous submit as pending verification instead of failure', () => {
 render(<RunHistory runs={[{id:3,task_id:3,account:'u',name:'每日签到',revision:1,date:'2026-10-26',scheduled_time:'12:30',status:'pending_verification',message:'请求超时，无法确认是否已提交',stage:'sign',failure_kind:'timeout',verification_attempts:1}]} />);
 expect(screen.getByText('待核对')).toBeInTheDocument();
 expect(screen.getByText(/复核：1 次/)).toBeInTheDocument();
 expect(screen.queryByText('失败')).not.toBeInTheDocument();
});
