import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { RunHistory } from './RunHistory';
it.each(['failed','unknown','skipped','unexpected'])('shows unsuccessful terminal status %s as failure', status => {
 render(<RunHistory runs={[{id:1,task_id:1,account:'u',name:'每日签到',revision:1,date:'2026-10-23',scheduled_time:'20:15',status,message:''}]} />);
 expect(screen.getByText('失败')).toBeInTheDocument();
 expect(screen.queryByText('待核对')).not.toBeInTheDocument();
});
