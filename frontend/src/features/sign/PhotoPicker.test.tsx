import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { expect, it, vi } from 'vitest';
import { PhotoPicker } from './PhotoPicker';
it('删除与卸载释放所有object URL，StrictMode不会遗留缩略图地址', async () => {
  let sequence = 0; const create = vi.fn(() => `blob:fixture-${++sequence}`); const revoke = vi.fn();
  vi.stubGlobal('URL', class extends URL { static createObjectURL = create; static revokeObjectURL = revoke; });
  const a = new File(['a'], 'a.png', { type: 'image/png' }); const b = new File(['b'], 'b.png', { type: 'image/png' }); const onChange = vi.fn();
  const { rerender, unmount } = render(<StrictMode><PhotoPicker photos={[a, b]} disabled={false} onChange={onChange} /></StrictMode>);
  await screen.findByAltText('a.png'); const url = screen.getByAltText('a.png').getAttribute('src');
  fireEvent.click(screen.getByRole('button', { name: '移除照片 a.png' })); expect(onChange).toHaveBeenCalledWith([b]);
  rerender(<StrictMode><PhotoPicker photos={[b]} disabled={false} onChange={onChange} /></StrictMode>);
  await waitFor(() => expect(revoke).toHaveBeenCalledWith(url)); unmount();
  expect(new Set(revoke.mock.calls.map(call => call[0]))).toEqual(new Set(create.mock.results.map(result => result.value)));
});
