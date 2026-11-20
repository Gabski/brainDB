// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { BrainNode } from '../../api/types';
import { isConceptNode } from '../../model/graph';
import { Icon } from '../common/Icon';

export type NodeAction =
  | 'focus'
  | 'toggleFocus'
  | 'test'
  | 'suggest'
  | 'path'
  | 'link'
  | 'linkExternal'
  | 'root'
  | 'near'
  | 'boost'
  | 'weaken'
  | 'supersede'
  | 'pin'
  | 'forget';

/** Actions for the selected node. */
export function ActionBar({ node, inFocus, onAction, busy }: { node: BrainNode; inFocus: boolean; onAction: (a: NodeAction) => void; busy?: boolean }) {
  const concept = isConceptNode(node);
  const B = ({ a, children, primary, danger }: { a: NodeAction; children: React.ReactNode; primary?: boolean; danger?: boolean }) => (
    <button className={'btn' + (primary ? ' primary' : '') + (danger ? ' danger' : '')} disabled={busy} onClick={() => onAction(a)}>
      {children}
    </button>
  );
  return (
    <div className="actions" role="toolbar" aria-label="Node actions">
      <B a="focus" primary>
        <Icon name="focus" /> Light as focus
      </B>
      <B a="toggleFocus">{inFocus ? <><Icon name="minus" /> Remove from focus</> : <><Icon name="add" /> Add to focus</>}</B>
      <B a="test"><Icon name="test" /> Test links</B>
      <B a="suggest"><Icon name="discover" /> Suggest links</B>
      <B a="path"><Icon name="path" /> Path to…</B>
      <B a="link"><Icon name="link" /> Link to…</B>
      <B a="linkExternal"><Icon name="source" /> Link external…</B>
      <B a="root"><Icon name="tree" /> Tree root</B>
      <B a="near"><Icon name="ruler" /> Find similar</B>
      {!concept && !node.external && (
        <>
          <B a="boost"><Icon name="boost" /> Boost</B>
          <B a="weaken"><Icon name="weaken" /> Weaken</B>
          <B a="supersede"><Icon name="supersede" /> Supersede by…</B>
          <B a="pin"><Icon name="pin" /> {node.pinned ? 'Unpin' : 'Pin'}</B>
          <B a="forget" danger>
            <Icon name="trash" /> Forget
          </B>
        </>
      )}
    </div>
  );
}
