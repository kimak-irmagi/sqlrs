import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { parse } from 'yaml';

test('CT12: native host attribution uses locked JDBC and the actual production fixture', () => {
  const workflow = parse(fs.readFileSync('.github/workflows/liquibase-cost-probe.yml', 'utf8'));
  const steps = workflow.jobs.probe.steps;
  const acquisition = steps.find(step => step.name === 'Acquire public native fixture');
  assert.match(acquisition.run, /docker pull postgres:17/);
  assert.match(acquisition.run, /node scripts\/fetch-sql-scripts.mjs --lock --source-prefix jdbc-postgresql-/);
  const native = steps.find(step => step.name === 'Qualify native host attribution and unchanged prepare');
  assert.equal(native['working-directory'], 'backend/local-engine-go');
  assert.equal(native.env.SQLRS_LIQUIBASE_DOCKER_E2E, '1');
  assert.equal(native.env.SQLRS_PREPARE_COST, '0');
  assert.equal(native.env.SQLRS_JDBC_JAR_PATH, '${{ github.workspace }}/dist/deps/postgresql-42.7.13.jar');
  assert.match(native.run, /TestCostMainPathNamedAttributionDocker\|TestMainPathLiquibasePrepareDocker/);
  assert.match(native.run, /-v -count=1 -timeout=8m/);
  assert.doesNotMatch(native.run, /SQLRS_LIQUIBASE_RELEASED_BENCH|strace|run-cohort/);
});

test('CT12: native intermediate results do not weaken completed-operation or data controls', () => {
  const source = fs.readFileSync('backend/local-engine-go/internal/prepare/cost_native_requirements_test.go', 'utf8');
  assert.match(source, /t.Setenv\("SQLRS_PREPARE_COST", "1"\)/);
  assert.match(source, /fields\[5\] != string\(preparecost.DockerCommand\) && fields\[10\] != string\(preparecost.Succeeded\)/);
  assert.match(source, /mainPathAssertRows\(t, f.ctx, status.Result.DSN, 3, 2\)/);
  assert.match(source, /observed\["prepare.worker"\] != 1/);
  assert.match(source, /len\(f.rt.runners\) != 3/);
  assert.match(source, /f.rt.runners\[i\] != f.rt.runners\[0\]/);
  assert.match(source, /f.rt.databases\[i\] != f.rt.databases\[0\]/);
  assert.match(source, /native temporary container leaked before fixture cleanup/);
  assert.match(source, /manager.DeleteInstance/);
});
