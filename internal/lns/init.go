package lns

func init() {
	// Register LNS handlers (singletons)
	RegisterLNSHandler(LNSTypeChirpStack, &ChirpStackHandler{})
	RegisterLNSHandler(LNSTypeTTN, &TTNHandler{})
	RegisterLNSHandler(LNSTypeHelium, &HeliumHandler{})
}
