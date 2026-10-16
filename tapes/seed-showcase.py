#!/usr/bin/env python3
"""Write fictional native sessions to a fresh recording workspace."""

import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
import sys


def write_jsonl(path, records):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(json.dumps(record) + "\n" for record in records))


def seed(root):
    config_dir = root / "config"
    config_dir.mkdir()  # Refuse to overwrite an existing recording setup.
    codex_root = root / ".codex" / "sessions"
    claude_root = root / ".claude" / "projects"
    codex_root.mkdir(parents=True)
    claude_root.mkdir(parents=True)
    (config_dir / "config.json").write_text(json.dumps({
        "theme": "green",
        "disable_default_roots": True,
        "roots": [
            {"provider": "codex", "path": str(codex_root)},
            {"provider": "claude-code", "path": str(claude_root)},
        ],
    }, indent=2) + "\n")

    # Relative dates keep the default seven-day overview populated on reruns.
    now = datetime.now(timezone.utc).replace(microsecond=0) - timedelta(minutes=15)
    sessions = [
        ("codex", "atlas", 0, "Make search feel instant",
         "Add keyboard navigation to the command palette. Keep results fast and preserve the current selection.",
         "The palette now supports arrow keys, Enter, and Escape. Search is debounced and the selection stays stable while results update."),
        ("claude-code", "harbor", 1, "Fix the flaky checkout test",
         "Find out why the checkout test fails intermittently and make it deterministic.",
         "The test was racing the payment callback. It now waits for the receipt state instead of a fixed timeout."),
        ("codex", "atlas", 24, "Add dark mode to the dashboard",
         "Add a dark palette with accessible text contrast and a saved theme preference.",
         "Dark mode is wired to the theme selector. The preference is saved and charts use the same semantic colours."),
        ("claude-code", "fieldnotes", 27, "Polish the onboarding flow",
         "Simplify the first-run checklist and make the next action clear.",
         "The welcome screen now has three short steps, a progress indicator, and a single primary action."),
        ("codex", "harbor", 48, "Retry failed background uploads",
         "Add bounded retries to the upload worker without duplicating completed uploads.",
         "The worker uses exponential backoff and an idempotency key. Retries stop after three attempts."),
        ("claude-code", "fieldnotes", 72, "Document the release checklist",
         "Write a short release checklist covering versioning, smoke tests, and rollback.",
         "The release guide now includes the exact build commands, a smoke-test checklist, and the rollback procedure."),
    ]
    names = []
    for number, (provider, project, hours, title, request, answer) in enumerate(sessions, 1):
        session_id = f"00000000-0000-4000-8000-{number:012d}"
        cwd = root / "projects" / project
        cwd.mkdir(parents=True, exist_ok=True)
        start = now - timedelta(hours=hours)

        def timestamp(seconds):
            return (start + timedelta(seconds=seconds)).isoformat().replace("+00:00", "Z")

        if provider == "codex":
            def event(kind, payload, seconds):
                return {"timestamp": timestamp(seconds), "type": kind, "payload": payload}

            def message(role, text, seconds):
                return event("response_item", {
                    "type": "message", "role": role,
                    "content": [{"type": "input_text" if role == "user" else "output_text", "text": text}],
                }, seconds)

            records = [
                event("session_meta", {"id": session_id, "timestamp": timestamp(0), "cwd": str(cwd),
                      "source": "cli", "git": {"branch": "feature/keyboard-search" if number == 1 else "main"}}, 0),
                event("turn_context", {"model": "gpt-6.1", "effort": "high", "cwd": str(cwd)}, 1),
                message("user", request, 5),
                message("assistant", "I will check the existing behaviour, make the change, and run the focused tests.", 15),
                event("response_item", {"type": "function_call", "name": "exec_command", "call_id": "demo-check",
                      "arguments": json.dumps({"cmd": "npm test -- --run", "workdir": str(cwd)})}, 90),
                event("response_item", {"type": "function_call_output", "call_id": "demo-check",
                      "output": "Tests: 24 passed. No failures."}, 100),
                message("user", "Keep the shortcuts visible so they are easy to discover.", 120),
                message("assistant", answer + " Shortcut hints are shown next to each action. All 24 focused tests pass.", 180),
                event("event_msg", {"type": "token_count", "info": {"total_token_usage": {
                    "input_tokens": 18400, "cached_input_tokens": 12800, "output_tokens": 2100,
                    "reasoning_output_tokens": 640}}}, 181),
                event("event_msg", {"type": "task_complete", "duration_ms": 180000}, 182),
            ]
            write_jsonl(codex_root / f"rollout-{session_id}.jsonl", records)
            names.append({"id": session_id, "thread_name": title})
        else:
            def message(role, text, seconds):
                content = {"role": role, "content": text}
                if role == "assistant":
                    content.update({"id": f"demo-response-{number}", "model": "claude-sonnet-4-5",
                                    "stop_reason": "end_turn", "usage": {"input_tokens": 6200,
                                    "cache_read_input_tokens": 4100, "cache_creation_input_tokens": 0,
                                    "output_tokens": 980}})
                return {"type": role, "sessionId": session_id, "timestamp": timestamp(seconds),
                        "cwd": str(cwd), "uuid": f"demo-{number}-{seconds}", "isSidechain": False,
                        "gitBranch": "main", "message": content}

            write_jsonl(claude_root / project / f"{session_id}.jsonl", [
                {"type": "summary", "sessionId": session_id, "summary": title,
                 "timestamp": timestamp(0), "cwd": str(cwd)},
                message("user", request, 5),
                message("assistant", answer, 180),
            ])
    write_jsonl(root / ".codex" / "session_index.jsonl", names)


if __name__ == "__main__":
    seed(Path(sys.argv[1]).resolve())
