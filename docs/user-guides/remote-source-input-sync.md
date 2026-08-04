# Remote Source Input Sync

This guide describes how `sqlrs` sends local source files to a remote Taidon
backend for file-bearing prepare and cache-explain commands.

Remote source sync is automatic profile behavior. It does not add a separate
command that users must run before every prepare request.

## Commands

The existing command syntax is unchanged:

```text
sqlrs prepare:psql [--image <image-id>] [--] [psql-args...]
sqlrs prepare:lb [--image <image-id>] [--] [liquibase-args...]
sqlrs plan:psql [--image <image-id>] [--] [psql-args...]
sqlrs plan:lb [--image <image-id>] [--] [liquibase-args...]
sqlrs cache explain prepare:psql [--image <image-id>] [--] [psql-args...]
sqlrs cache explain prepare:lb [--image <image-id>] [--] [liquibase-args...]
```

When the selected profile is `local`, the CLI does not transfer files. The
local engine reads the workspace filesystem directly.

When the selected profile is `remote`, the CLI attaches a source manifest to
file-bearing prepare and cache-explain requests, handles recoverable missing
input responses, uploads only server-requested content blobs, and retries the
original request. Successful command stdout stays unchanged; source-sync
progress is written to stderr.

The CLI binds path-bearing arguments to absolute native paths before sending a
request. Remote source sync preserves those paths as logical client coordinates;
the server does not try to open them on its own filesystem. The attached
workspace context identifies the absolute client workspace root and effective
working directory so admission can map arguments and imports into the
workspace-relative manifest namespace.

Remote requests always use the invoking host's native path syntax. In
particular, configuring a Windows profile to run its local engine in WSL does
not rewrite remote request paths from `C:\...` to `/mnt/c/...`; WSL path
conversion is scoped to local-engine execution only.

## Profile Configuration

Remote source sync is enabled by default for remote profiles.

```yaml
profiles:
  remote:
    mode: remote
    endpoint: https://api.taidon.dev
    sourceSync:
      mode: auto
      maxRounds: 8
```

`sourceSync.mode` values:

| Value | Behavior |
| --- | --- |
| `auto` | Attach source manifests and upload only server-reported missing blobs. This is the default for remote profiles. |
| `off` | Do not attach manifests or upload source blobs. File-bearing remote requests may fail with source-input errors. |

`sourceSync.maxRounds` bounds the missing-input retry loop. Reaching the limit
is a client-side failure that reports the last server response.

## Source Manifest

A source manifest is an execution-scoped description of source files and
directories the client can provide. Manifest paths are workspace-relative,
slash-separated paths. Absolute host paths are never sent in the manifest.

Conceptual shape:

```yaml
source_manifest:
  workspace_ref:
    root_id: default
    root_path: 'C:\work\project'
    work_dir: 'C:\work\project\db'
  files:
    db/changelog/master.xml: sha256:...
    db/changelog/001.sql: sha256:...
  directories:
    db/changelog:
      entries:
        - name: master.xml
          kind: file
        - name: changes
          kind: directory
```

`root_path` and `work_dir` use the client's native absolute path syntax. They
bind absolute request arguments to manifest keys and are not server filesystem
paths. The server remains authoritative for format-specific dependency
traversal. The client can send a narrow seed manifest, then expand it only when
the server asks for file hashes, directory listings, or content blobs.

The first request may contain an empty manifest. The server rebases absolute
path-bearing arguments against the logical client workspace context, derives
the initial required entries, and returns workspace-relative
`source_inputs_missing` paths. It must not create a prepare job until source
admission has succeeded.

## Missing Input Loop

If the server needs more source information, it returns a recoverable
`409 source_inputs_missing` response:

```json
{
  "code": "source_inputs_missing",
  "message": "source inputs are missing",
  "missing_manifest_entries": [
    {
      "path": "db/changelog/changes",
      "kind": "directory_listing"
    },
    {
      "path": "db/changelog/changes/002.sql",
      "kind": "file_hash"
    }
  ],
  "missing_blobs": [
    {
      "path": "db/changelog/master.xml",
      "hash": "sha256:..."
    }
  ]
}
```

The CLI handles the response by:

1. adding requested file hashes and directory listings to the next manifest;
2. uploading only blobs listed in `missing_blobs`;
3. retrying the original prepare or cache-explain request with the expanded
   manifest.

The command fails when a requested local path is unavailable, a local hash does
not match the server-requested hash, a blob upload is rejected, the retry limit
is reached, or the server returns a non-recoverable error.

Source synchronization follows the common CLI progress presentation model and
does not add a new command-line option:

- with `-v/--verbose`, every source-sync progress event is written as a separate
  stderr line;
- without verbose output on an interactive terminal, one delayed spinner line
  shows the current operation and is updated in place;
- without verbose output on a non-interactive stderr, no spinner or ANSI control
  sequences are emitted;
- successful command stdout remains unchanged in every mode.

Progress events cover sync start, recoverable-round requests, each requested
file hash or directory listing, blob upload start/progress/completion, request
retry, and final completion. Blob events include the workspace-relative path,
actual transferred bytes, total bytes when known, ordinal within the requested
blob set, and a shortened digest. They never include source contents or an
absolute host path.

Example verbose output:

```text
source sync: started
source sync: round 1 requested manifest=3 blobs=0
source sync: hashing flights/schema.sql
source sync: hashed flights/schema.sql sha256:dbfc71aa...
source sync: round 2 requested manifest=0 blobs=3
source sync: uploading flights/schema.sql sha256:dbfc71aa... 12.4/20.1 KiB (1/3)
source sync: uploaded flights/schema.sql sha256:dbfc71aa... 20.1 KiB (1/3)
source sync: complete files=3 listings=0 uploaded=3 bytes=41.7 KiB
```

The interactive spinner renders the same event stream as a current-operation
line, for example:

```text
source sync: uploading schema.sql 12.4/20.1 KiB (2/3) |
```

Byte progress represents bytes actually consumed by the HTTP upload body, not
merely files selected for transfer. A final verbose summary therefore provides
client-side evidence of how many blobs and bytes were sent. No verbose
source-sync event means that the server accepted the initial request without a
recoverable `source_inputs_missing` response.

Each deployment defines a maximum size for one source blob. A file within that
published limit is uploaded as one content-addressed blob; a larger file fails
with `413 source_blob_too_large`. Supported deployments must configure the limit
high enough for the maintained file-backed examples.

## Ref Mode

For `--ref-mode worktree`, source sync reads from the projected detached
worktree.

For `--ref-mode blob`, source sync reads from the Git object-backed filesystem.
It must not fall back to the caller's current worktree for source bytes.
