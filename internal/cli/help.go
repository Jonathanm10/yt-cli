package cli

import "strings"

func helpText(path []string) string {
	switch strings.Join(path, " ") {
	case "":
		return rootHelpText()
	case "auth":
		return authHelpText()
	case "auth login":
		return authLoginHelpText()
	case "auth status":
		return authStatusHelpText()
	case "auth logout":
		return authLogoutHelpText()
	case "profile":
		return profileHelpText()
	case "profile list":
		return profileListHelpText()
	case "profile use":
		return profileUseHelpText()
	case "project":
		return projectHelpText()
	case "project list":
		return projectListHelpText()
	case "issue":
		return issueHelpText()
	case "issue view":
		return issueViewHelpText()
	case "issue search":
		return issueSearchHelpText()
	case "issue create":
		return issueCreateHelpText()
	case "issue update":
		return issueUpdateHelpText()
	case "issue comment":
		return issueCommentHelpText()
	case "issue transition":
		return issueTransitionHelpText()
	case "issue assign":
		return issueAssignHelpText()
	case "issue attach":
		return issueAttachHelpText()
	case "workitem":
		return workItemHelpText()
	case "workitem view":
		return workItemViewHelpText()
	default:
		if len(path) <= 1 {
			return rootHelpText()
		}
		return helpText(path[:1])
	}
}

func rootHelpText() string {
	return `yt-cli - YouTrack CLI for terminal and automation

Usage:
  yt-cli <command> [flags]
  yt-cli help [command]

Commands:
  auth      Authenticate and manage local tokens and profiles
  profile   List or switch local profiles
  project   List projects
  issue     View, search, create, and update issues
  workitem  Compatibility alias for "issue view"

Examples:
  yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud
  yt-cli project list --profile sandbox
  yt-cli issue view ABC-123 --profile sandbox
  yt-cli issue search --query 'in:ABC #Unresolved' --top 5 --profile sandbox

Use "yt-cli <command> --help" for command-specific help.
`
}

func authHelpText() string {
	return `Usage:
  yt-cli auth <login|status|logout> [flags]
  yt-cli auth help <login|status|logout>

Commands:
  login     Authenticate a profile and store a token locally
  status    Validate the active token and show the current user
  logout    Remove the stored token or delete the profile

Examples:
  yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud
  printf '%s' "$YT_TOKEN" | yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud --token-stdin
  yt-cli auth status --profile sandbox
  yt-cli auth logout --profile sandbox --delete-profile

Use "yt-cli auth <command> --help" for details on a specific auth command.
`
}

func authLoginHelpText() string {
	return `Usage:
  yt-cli auth login --base-url URL [--profile NAME] [--default-project KEY] [--token-stdin] [flags]

Options:
  --base-url URL          YouTrack base URL
  --profile NAME          profile name (default: active profile or "default")
  --default-project KEY   default project saved with the profile
  --token-stdin           read the permanent token from stdin instead of prompting
  --json-errors           emit machine-readable errors
  --debug                 emit debug logs to stderr

Examples:
  yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud
  printf '%s' "$YT_TOKEN" | yt-cli auth login --profile sandbox --base-url https://your-instance.youtrack.cloud --token-stdin
`
}

func authStatusHelpText() string {
	return `Usage:
  yt-cli auth status [--profile NAME] [--base-url URL] [--json-errors] [--debug]

Examples:
  yt-cli auth status
  yt-cli auth status --profile sandbox
`
}

func authLogoutHelpText() string {
	return `Usage:
  yt-cli auth logout [--profile NAME] [--delete-profile] [--json-errors] [--debug]

Options:
  --delete-profile   delete the profile and its stored token

Examples:
  yt-cli auth logout --profile sandbox
  yt-cli auth logout --profile sandbox --delete-profile
`
}

func profileHelpText() string {
	return `Usage:
  yt-cli profile <list|use> [flags]
  yt-cli profile help <list|use>

Commands:
  list   list configured profiles
  use    make a profile active

Examples:
  yt-cli profile list
  yt-cli profile use sandbox
`
}

func profileListHelpText() string {
	return `Usage:
  yt-cli profile list [--json-errors] [--debug]

Examples:
  yt-cli profile list
`
}

func profileUseHelpText() string {
	return `Usage:
  yt-cli profile use NAME [--json-errors] [--debug]

Examples:
  yt-cli profile use sandbox
`
}

func projectHelpText() string {
	return `Usage:
  yt-cli project list [--query TEXT] [flags]
  yt-cli project help list

Commands:
  list   list projects visible to the authenticated user

Examples:
  yt-cli project list --profile sandbox
  yt-cli project list --query ops --profile sandbox
`
}

func projectListHelpText() string {
	return `Usage:
  yt-cli project list [--query TEXT] [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Options:
  --query TEXT   filter projects by text
  --raw          emit the raw API payload envelope

Examples:
  yt-cli project list --profile sandbox
  yt-cli project list --query sample --profile sandbox --raw
`
}

func issueHelpText() string {
	return `Usage:
  yt-cli issue <view|search|create|update|comment|transition|assign|attach> [flags]
  yt-cli issue help <command>

Commands:
  view         show one issue
  search       search issues with a YouTrack query
  create       create an issue
  update       update summary, description, or custom fields
  comment      add a comment to an issue
  transition   change the issue state
  assign       assign an issue to a user
  attach       upload one or more attachments

Examples:
  yt-cli issue view ABC-123 --profile sandbox
  yt-cli issue search --query 'in:ABC #Unresolved' --top 5 --profile sandbox
  yt-cli issue create --project ABC --summary 'Example issue' --profile sandbox
  yt-cli issue comment ABC-123 --text 'Investigating now' --profile sandbox

Use "yt-cli issue <command> --help" for flags and examples for a specific issue command.
`
}

func issueViewHelpText() string {
	return `Usage:
  yt-cli issue view ISSUE_ID [--profile NAME] [--base-url URL] [--fields PRESET] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue view ABC-123 --profile sandbox
  yt-cli issue view ABC-123 --profile sandbox --fields id,idReadable
`
}

func issueSearchHelpText() string {
	return `Usage:
  yt-cli issue search --query TEXT [--top N] [--skip N] [--profile NAME] [--base-url URL] [--fields PRESET] [--raw] [--json-errors] [--debug]

Options:
  --query TEXT   YouTrack query
  --top N        page size (default: 25)
  --skip N       offset (default: 0)

Examples:
  yt-cli issue search --query 'in:ABC #Unresolved' --top 5 --profile sandbox
  yt-cli issue search --query 'for: me sort by: updated desc' --skip 10 --profile sandbox --raw
`
}

func issueCreateHelpText() string {
	return `Usage:
  yt-cli issue create --summary TEXT [--project KEY] [--description TEXT] [--field NAME=VALUE ...] [--attach PATH ...] [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Options:
  --project KEY        project short name; falls back to the profile default project
  --summary TEXT       issue summary (required)
  --description TEXT   issue description
  --field NAME=VALUE   repeatable custom field assignment
  --attach PATH        repeatable attachment path

Examples:
  yt-cli issue create --project ABC --summary 'Example issue' --description 'Created from yt-cli' --profile sandbox
  yt-cli issue create --summary 'Uses default project' --field Priority=Critical --profile sandbox
`
}

func issueUpdateHelpText() string {
	return `Usage:
  yt-cli issue update ISSUE_ID [--summary TEXT] [--description TEXT] [--field NAME=VALUE ...] [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue update ABC-123 --summary 'Updated summary' --profile sandbox
  yt-cli issue update ABC-123 --field Priority=Major --field State='In Progress' --profile sandbox
`
}

func issueCommentHelpText() string {
	return `Usage:
  yt-cli issue comment ISSUE_ID --text TEXT [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue comment ABC-123 --text 'Investigating now' --profile sandbox
`
}

func issueTransitionHelpText() string {
	return `Usage:
  yt-cli issue transition ISSUE_ID --state TEXT [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue transition ABC-123 --state 'In Progress' --profile sandbox
`
}

func issueAssignHelpText() string {
	return `Usage:
  yt-cli issue assign ISSUE_ID --user LOGIN [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue assign ABC-123 --user jane.doe --profile sandbox
`
}

func issueAttachHelpText() string {
	return `Usage:
  yt-cli issue attach ISSUE_ID PATH [PATH ...] [--profile NAME] [--base-url URL] [--raw] [--json-errors] [--debug]

Examples:
  yt-cli issue attach ABC-123 ./artifact.txt ./log.txt --profile sandbox
`
}

func workItemHelpText() string {
	return `Usage:
  yt-cli workitem view ISSUE_ID [flags]
  yt-cli workitem help view

Commands:
  view   compatibility alias for "yt-cli issue view"

Examples:
  yt-cli workitem view ABC-123 --profile sandbox
`
}

func workItemViewHelpText() string {
	return `Usage:
  yt-cli workitem view ISSUE_ID [--profile NAME] [--base-url URL] [--fields PRESET] [--raw] [--json-errors] [--debug]

Notes:
  This command is a compatibility alias for "yt-cli issue view".

Examples:
  yt-cli workitem view ABC-123 --profile sandbox
`
}
