// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { useState } from 'react';
import { CodeEditor } from '../components/common/CodeEditor';
import { bdqlCompletions } from '../model/completions';
import { fixtureConfigErrorsYaml, fixtureTemplates } from '../fixtures';
import { Frame, g } from './helpers';

const meta: Meta = { title: 'brainDB/Editor/CodeEditor' };
export default meta;
type S = StoryObj;

const BDQL = `-- BDQL with every token class
USE sso;
SELECT id, kind, text, fields.users, score
  FROM memory@*
  WHERE kind IN (record, fact) AND fields.users > 100 AND text ~ 'wave'
        AND lit AND NOT superseded AND HAS person AND entities.date BETWEEN '2026-11-01' AND '2026-11-30'
  NEAR "rollout waves" MODE hybrid
  ORDER BY score DESC LIMIT 10 FORMAT md;
REMEMBER task "Verify guest access" CONCEPTS "Security" LINK TO k175 WEIGHT 0.8 AS T;
LINK $T TO lab:x204 WEIGHT 0.7;
CONNECTOR anchor "Kim Lee" KEY 'owner:kim-lee' CONNECT r191, r193 GAIN 0.95 TRANSPARENT AS H;
FILL $$
project: "project name"
$$ AS yaml OUTPUT md;
`;

function Ed(props: { value: string; lang: 'bdql' | 'yaml' | 'compact'; markers?: { line: number; col: number; message: string }[]; ac?: string }) {
  const [v, setV] = useState(props.value);
  return (
    <Frame width={900}>
      <CodeEditor value={v} onChange={setV} lang={props.lang} markers={props.markers} completions={bdqlCompletions(g, fixtureTemplates, v)} initialCompletion={props.ac ? { prefix: props.ac } : undefined} minRows={14} onRun={() => {}} onRunAll={() => {}} />
    </Frame>
  );
}

export const BdqlHighlighting: S = { render: () => <Ed value={BDQL} lang="bdql" /> };
export const AutocompleteOpen: S = { render: () => <Ed value={'SELECT id, text FROM memory WHERE kind = rec'} lang="bdql" ac="rec" /> };
export const AutocompleteIds: S = { render: () => <Ed value={'INSPECT d12'} lang="bdql" ac="d12" /> };
export const AutocompleteFields: S = { render: () => <Ed value={'SELECT fields.'} lang="bdql" ac="fields." /> };
export const LintError: S = {
  render: () => (
    <Ed
      value={'SELECT id FROM memory;\n\nSELEKT id FROM memory;\n\nSELECT id FROM memory WHERE kind = tsk;'}
      lang="bdql"
      markers={[
        { line: 3, col: 1, message: 'unknown keyword SELEKT (did you mean SELECT?)' },
        { line: 5, col: 36, message: 'unknown kind "tsk" (did you mean task?)' },
      ]}
    />
  ),
};
export const YamlWithErrors: S = {
  render: () => (
    <Ed
      value={fixtureConfigErrorsYaml}
      lang="yaml"
      markers={[
        { line: 5, col: 1, message: 'embedding.use: must be raw or enriched' },
        { line: 7, col: 1, message: 'unknown key "kindz" (did you mean "kinds"?)' },
      ]}
    />
  ),
};
export const CompactQuery: S = { render: () => <Ed value={'kind:record users>100 owner~kim "free text" near:"rollout risk" sort:-users limit:10 select:id,text,fields.users'} lang="compact" /> };
