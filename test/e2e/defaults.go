package e2e

var defaultResources = Resources{
	MemoryRequest: "200Mi",
	MemoryLimit:   "200Mi",
	CPU:           "300m",
	Volume:        "1Gi",
}

var maxValidatorResources = Resources{
	MemoryRequest: "10Gi",
	MemoryLimit:   "12Gi",
	CPU:           "6",
	Volume:        "1Gi",
}

var maxTxsimResources = Resources{
	MemoryRequest: "1Gi",
	MemoryLimit:   "1Gi",
	CPU:           "2",
	Volume:        "1Gi",
}
