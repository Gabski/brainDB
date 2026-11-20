// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { Icon, ICONS, type IconName } from '../components/common/Icon';
import { Frame } from './helpers';

const meta: Meta = { title: 'brainDB/Foundations/Icons' };
export default meta;
type S = StoryObj;

/** Every icon name used by the UI (lucide-react, ISC), at the three grid sizes. */
export const AllIcons: S = {
  render: () => (
    <Frame width={900}>
      <table className="tbl">
        <thead>
          <tr>
            <th>name</th>
            <th>14</th>
            <th>16</th>
            <th>20</th>
            <th>in a button</th>
          </tr>
        </thead>
        <tbody>
          {(Object.keys(ICONS) as IconName[]).map((n) => (
            <tr key={n}>
              <td className="mono">{n}</td>
              <td><Icon name={n} size={14} /></td>
              <td><Icon name={n} size={16} /></td>
              <td><Icon name={n} size={20} /></td>
              <td><button className="btn"><Icon name={n} /> {n}</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </Frame>
  ),
};
export const Logo: S = { render: () => <Frame><div className="logo" style={{ fontSize: 18 }}><img src="./logo.svg" alt="" width={32} height={32} /> brainDB</div></Frame> };
