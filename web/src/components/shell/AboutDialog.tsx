// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { Modal } from '../common/Modal';

/** About brainDB: version (from /api/health), copyright, license, attribution. */
export function AboutDialog({ version, onClose }: { version?: string | null; onClose: () => void }) {
  return (
    <Modal title="About brainDB" onClose={onClose} width={460}>
      <div className="col">
        <div className="logo" style={{ fontSize: 18 }}>
          <img src="./logo.svg" alt="" width={28} height={28} /> brainDB <small>{version ? 'v' + version : 'version unknown'}</small>
        </div>
        <div className="small">A long-term memory database for AI agents and people: a weighted graph of memories, concepts and connectors, where only the lit slice — the current context — is handed to a model.</div>
        <table className="props">
          <tbody>
            <tr>
              <th>Copyright</th>
              <td className="plain">© 2026 Gabriel Koziestański</td>
            </tr>
            <tr>
              <th>License</th>
              <td>Apache-2.0</td>
            </tr>
          </tbody>
        </table>
        <div className="xs muted">
          Licensed under the Apache License, Version 2.0. Redistributions must keep the NOTICE file, which credits Gabriel Koziestański as the original author.
        </div>
        <div className="row">
          <a className="link" href="./LICENSE" target="_blank" rel="noreferrer">
            LICENSE
          </a>
          <a className="link" href="./NOTICE" target="_blank" rel="noreferrer">
            NOTICE
          </a>
          <a className="link" href="https://www.apache.org/licenses/LICENSE-2.0" target="_blank" rel="noreferrer">
            apache.org/licenses/LICENSE-2.0
          </a>
        </div>
      </div>
    </Modal>
  );
}
