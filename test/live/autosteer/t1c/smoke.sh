#!/bin/sh
# smoke.sh NAME: the channel smoke before any paid T1c run. One `claude -p` session of one turn on the haiku model through
# the G arm's hooks (run.sh G, slice t1c, smoke-prompt.txt in place of the frozen prompt, so the graph brief renders and
# the model stops), then channel.py over the run. Proves, at a few cents, that a note built at UserPromptSubmit reaches
# the session as additionalContext under the corrected hook.sh (B448) and lands in the transcript as the attachment
# Claude Code shows the model. The judge never runs on a smoke run and no row is written. Runs nothing without
# RK_T1C_GO=yes, which only the owner's word sets. Environment: run.sh's (RK_CONFIG_DIR, RK_BIN_G, RK_PGRAPH_G,
# RK_KNOWLEDGE, RK_GO_TOOLCHAIN_BIN, RK_RUNS).
set -u
NAME=${1:?NAME}
HERE=$(cd "$(dirname "$0")" && pwd)
[ "${RK_T1C_GO:-}" = yes ] || { echo "smoke.sh: RK_T1C_GO is not yes; nothing runs without the owner's word" >&2; exit 2; }
RK_RUNS=${RK_RUNS:-/tmp/rk-autosteer/runs}
RK_SLICE=t1c RK_PROMPT_FILE=$HERE/smoke-prompt.txt RK_MODEL=claude-haiku-4-5-20251001 RK_MAX_TURNS=1 RK_BUDGET_USD=0.20 \
  sh "$HERE/../run.sh" G "$NAME" || { echo "smoke.sh: run.sh failed"; exit 1; }
python3 "$HERE/channel.py" "$RK_RUNS/$NAME" G
