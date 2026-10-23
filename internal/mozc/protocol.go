package mozc

import "google.golang.org/protobuf/encoding/protowire"

// Field numbers and enum values of Mozc 2.29.5160.102
// (src/protocol/commands.proto, candidates.proto and config.proto).

// Input fields.
const (
	inputType              protowire.Number = 1
	inputID                protowire.Number = 2
	inputKey               protowire.Number = 3
	inputCommand           protowire.Number = 4
	inputConfig            protowire.Number = 5
	inputRequestSuggestion protowire.Number = 14
)

// Input.CommandType values.
const (
	commandCreateSession = 1
	commandDeleteSession = 2
	commandSendKey       = 3
	commandSendCommand   = 5
	commandSetConfig     = 7
)

// SessionCommand fields and values.
const (
	sessionCommandType            protowire.Number = 1
	sessionCommandCompositionMode protowire.Number = 3

	sessionCommandTurnOnIME = 22
	compositionModeHiragana = 1
)

// KeyEvent fields and values.
const (
	keySpecialKey   protowire.Number = 3
	keyModifierKeys protowire.Number = 4
	keyKeyString    protowire.Number = 5
	keyInputStyle   protowire.Number = 6

	specialKeySpace = 4
	specialKeyLeft  = 6
	specialKeyRight = 7
	modifierShift   = 4
	inputStyleAsIs  = 1
)

// Output fields and values.
const (
	outputID                protowire.Number = 1
	outputPreedit           protowire.Number = 5
	outputConfig            protowire.Number = 9
	outputErrorCode         protowire.Number = 11
	outputAllCandidateWords protowire.Number = 14

	errorCodeSessionSuccess = 0
)

// Preedit fields and values.
const (
	preeditSegmentGroup protowire.Number = 2
	segmentAnnotation   protowire.Number = 3
	segmentValue        protowire.Number = 4
	segmentKey          protowire.Number = 6

	annotationHighlight = 2
)

// CandidateList and CandidateWord fields.
const (
	candidateListCandidates protowire.Number = 2
	candidateWordID         protowire.Number = 1
	candidateWordValue      protowire.Number = 4
)

// Config fields and values.
const (
	configIncognitoMode        protowire.Number = 20
	configSessionKeymap        protowire.Number = 41
	configHistoryLearningLevel protowire.Number = 50

	sessionKeymapMSIME = 2
	historyNoHistory   = 2
)
