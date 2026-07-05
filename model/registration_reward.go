package model

type RegistrationRewardPolicy struct {
	SkipInitialQuota bool
	SkipInviteReward bool
}

func (p RegistrationRewardPolicy) withDefaults() RegistrationRewardPolicy {
	return p
}
