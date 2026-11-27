# TaskDeck interactive user questionnaire

`taskdeck_ask_user` is a local Agent Chat function for missing user decisions, not native COLLECT or PATCH. It is advertised in normal local Agent Chat requests only when function calling is verified for the selected model. Responses API and Chat Completions use the existing tool transport. Run Manager, shared-server mode and text-only fallback are not supported by this first implementation.

## Request

Send a single tool call containing 1–8 questions. Question IDs and option IDs must be unique in their scope, 1–40 ASCII letters/digits/underscore/hyphen, starting with a letter or digit. Text is UTF-8. Choice questions require 2–8 options, with no default selection. Ask in the user's language. Example:

```json
{
  "questions": [
    {
      "id": "platform",
      "prompt": "Which platform?",
      "type": "single_choice",
      "required": true,
      "options": [{"id":"android","label":"Android"},{"id":"ios","label":"iOS"}],
      "allow_comment": true
    },
    {
      "id": "features",
      "prompt": "Which features?",
      "type": "multiple_choice",
      "required": false,
      "options": [{"id":"logs","label":"Logs"},{"id":"tests","label":"Tests"}],
      "allow_comment": true
    },
    {
      "id": "details",
      "prompt": "Additional requirements?",
      "type": "text",
      "required": false,
      "options": [],
      "allow_comment": false
    }
  ]
}
```

## Result and lifecycle

The pending immutable questionnaire is stored in the conversation's existing tool ledger with status `pending` / session `waiting_tool`. No workspace operation or provider call occurs while waiting. Neither Auto apply nor Auto Continue may answer it. The owner presses Send answers; the server validates exact call ID and argument digest and the submitted answers, saves the result, and uses the existing once-only feedback journal to deliver a matching function_call_output. Cancel / Skip sends explicit CANCELLED with an empty answers array. No native approval is implied.

```json
{"status":"ANSWERED","answers":[{"question_id":"platform","selected":["android"],"text":"Version 14+"},{"question_id":"features","selected":["logs","tests"],"text":""},{"question_id":"details","selected":[],"text":"Keep existing data"}]}
```

Optional unanswered questions have empty selections/text. The answer result uses option IDs, not positional indexes. A submitted result is durable; pre-egress errors may be resumed via Send saved answers to AI. Ambiguous post-egress errors retain the original journal and must not silently replay. The form's unsent draft is memory-only per session/call and survives UI rerender and tab switches, not a full browser reload. The original pending questions survive server/browser restart. Limits: 24 KiB request arguments, 1,000 UTF-8 bytes per prompt, 400 per option label, 1,200 per answer text, 4,096 total serialized answer bytes. Do not solicit credentials/secrets. No arbitrary model HTML, shell execution, or approval bypass.
