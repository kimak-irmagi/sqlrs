import fs from 'node:fs';
import { selectRun } from './control.mjs';

const selection = selectRun(process.env.GITHUB_EVENT_NAME,
  JSON.parse(fs.readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8')), process.env.GITHUB_REPOSITORY);
if (!selection) throw new Error('Performance run is not opted in');
fs.appendFileSync(process.env.GITHUB_OUTPUT, `sha=${selection.sha}\nphase=${selection.phase}\n`);
