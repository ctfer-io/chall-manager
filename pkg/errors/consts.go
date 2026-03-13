package errors

const (
	// Domain of errors, in the scope of the service.
	Domain = "github.com/ctfer-io/chall-manager"

	// Follows all error reasons.

	// => Challenge errors (business layer)

	ReasonChallengeAlreadyExists = "CHALLENGE_ALREADY_EXISTS"
	ReasonChallengeNotFound      = "CHALLENGE_NOT_FOUND"
	ReasonChallengeExpired       = "CHALLENGE_EXPIRED"
	ReasonChallengeNoRenewal     = "CHALLENGE_NO_RENEWAL"
	ReasonChallengePoolerOOB     = "CHALLENGE_POOLER_OUT_OF_BOUNDS"
	ReasonChallengeInvalidUM     = "CHALLENGE_INVALID_UPDATE_MASK"

	// => Instance errors (business layer)

	ReasonInstanceAlreadyExists = "INSTANCE_ALREADY_EXISTS"
	ReasonInstanceNotFound      = "INSTANCE_NOT_FOUND"
	ReasonInstanceExpired       = "INSTANCE_EXPIRED"

	// => OCI/Scenario errors

	ReasonOCIMalformedRef         = "OCI_REFERENCE_MALFORMED"
	ReasonOCIInteraction          = "OCI_INTERACTION"
	ReasonOCINotFound             = "OCI_REFERENCE_NOT_FOUND"
	ReasonScenarioNonMatchingSpec = "SCENARIO_NON_MATCHING_SPECIFICATION"
	ReasonScenarioNotFound        = "SCENARIO_NOT_FOUND"
	ReasonScenarioPreprocess      = "SCENARIO_PREPROCESSING"
)
