package payment

// ExecuteStrand executes a strand using the two-pass algorithm.
//
// The algorithm works as follows:
// 1. Reverse Pass: Starting from the desired output, work backwards through the strand
//    calling Rev() on each step to determine how much input is needed to produce that output.
// 2. Find Limiting Step: The step that produces less output than requested is the "limiting" step.
// 3. Forward Pass: Starting from the limiting step, work forwards calling Fwd() on each step
//    to execute the actual transfer with the computed amounts.
//
// Parameters:
//   - baseView: The PaymentSandbox to clone for this strand execution
//   - strand: The sequence of steps to execute
//   - maxIn: Optional maximum input amount (nil means unlimited)
//   - requestedOut: The desired output amount
//
// Returns: StrandResult containing actual input/output and the sandbox with changes
func ExecuteStrand(
	baseView *PaymentSandbox,
	strand Strand,
	maxIn *EitherAmount,
	requestedOut EitherAmount,
) StrandResult {
	if len(strand) == 0 {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: nil,
			Inactive: true,
		}
	}

	// Create a sandbox for this strand execution
	sb := NewChildSandbox(baseView)
	ofrsToRm := make(map[[32]byte]bool)

	// === REVERSE PASS ===
	// Work backwards from desired output to determine required inputs
	result := executeReversePass(sb, baseView, strand, requestedOut, ofrsToRm)
	if !result.Success {
		return result
	}

	// Check if we hit a maxIn limit
	if maxIn != nil && result.In.Compare(*maxIn) > 0 {
		// Need to re-execute with limited input
		// Find how much output we can get with maxIn
		result = executeWithMaxIn(baseView, strand, *maxIn, ofrsToRm)
	}

	return result
}

// executeReversePass performs the reverse pass of the two-pass algorithm
func executeReversePass(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	strand Strand,
	requestedOut EitherAmount,
	ofrsToRm map[[32]byte]bool,
) StrandResult {
	// Start with requested output at the last step
	out := requestedOut
	limitingStep := -1

	// Work backwards through steps
	for i := len(strand) - 1; i >= 0; i-- {
		step := strand[i]

		// Call Rev to get how much input produces this output
		actualIn, actualOut := step.Rev(sb, afView, ofrsToRm, out)

		// Check if this step limited the flow
		if !step.EqualOut(actualOut, out) {
			limitingStep = i
		}

		// The input to this step is the output for the previous step
		out = actualIn
	}

	// Get cached results from first step
	firstStep := strand[0]
	cachedIn := firstStep.CachedIn()
	if cachedIn == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	// Get cached results from last step
	lastStep := strand[len(strand)-1]
	cachedOut := lastStep.CachedOut()
	if cachedOut == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	// If there's a limiting step, we need to re-execute with that step as starting point
	if limitingStep >= 0 {
		return executeWithLimitingStep(sb, afView, strand, limitingStep, ofrsToRm)
	}

	// No limiting step - Rev() already applied all changes (like rippled)
	// Reference: rippled StrandFlow.h - when no limiting step, only Rev() runs
	// Both DirectStep.Rev() and BookStep.Rev() apply changes via rippleCredit/consumeOffer

	// Calculate total offers used
	var offersUsed uint32
	for _, step := range strand {
		offersUsed += step.OffersUsed()
	}

	// Check if strand became inactive
	inactive := false
	for _, step := range strand {
		if step.Inactive() {
			inactive = true
			break
		}
	}

	return StrandResult{
		Success:    true,
		In:         *cachedIn,
		Out:        *cachedOut,
		Sandbox:    sb,
		OffsToRm:   ofrsToRm,
		OffersUsed: offersUsed,
		Inactive:   inactive,
	}
}

// executeWithLimitingStep re-executes when a limiting step was found
func executeWithLimitingStep(
	sb *PaymentSandbox,
	afView *PaymentSandbox,
	strand Strand,
	limitingStep int,
	ofrsToRm map[[32]byte]bool,
) StrandResult {
	// Reset sandbox state
	sb.Reset()

	// === SECOND REVERSE PASS ===
	// Re-execute reverse pass up to the limiting step
	limitStepCachedOut := strand[limitingStep].CachedOut()
	if limitStepCachedOut == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	out := *limitStepCachedOut

	// Execute reverse pass from limiting step backwards
	for i := limitingStep; i >= 0; i-- {
		step := strand[i]
		actualIn, _ := step.Rev(sb, afView, ofrsToRm, out)
		out = actualIn
	}

	// === FORWARD PASS ===
	// Execute forward from limiting step to the end
	limitStepCachedIn := strand[limitingStep].CachedIn()
	if limitStepCachedIn == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	in := *limitStepCachedIn

	// Rev() already applied changes for steps 0 to limitingStep (like rippled)
	// So we start the forward pass AFTER the limiting step

	// For the limiting step, get its output to use as input for subsequent steps
	// Reference: rippled StrandFlow.h line 225: EitherAmount stepIn(limitStepOut)
	limitStepCachedOutAfterRev := strand[limitingStep].CachedOut()
	if limitStepCachedOutAfterRev != nil {
		in = *limitStepCachedOutAfterRev
	}

	// Forward pass continues AFTER the limiting step (limitingStep + 1)
	// Reference: rippled StrandFlow.h line 226: for (auto i = limitingStep + 1; i < s; ++i)
	// The Fwd() pass applies changes for steps after the limiting step.
	for i := limitingStep + 1; i < len(strand); i++ {
		step := strand[i]

		// Execute forward - skip ValidFwd since we just ran Rev() with correct amounts
		actualIn, actualOut := step.Fwd(sb, afView, ofrsToRm, in)

		// Check consistency
		if step.IsZero(actualOut) && !step.IsZero(in) {
			return StrandResult{
				Success:  false,
				In:       ZeroXRPEitherAmount(),
				Out:      ZeroXRPEitherAmount(),
				Sandbox:  nil,
				OffsToRm: ofrsToRm,
				Inactive: true,
			}
		}

		_ = actualIn

		// The output becomes the input for the next step
		in = actualOut
	}

	// Get final results
	firstCachedIn := strand[0].CachedIn()
	lastCachedOut := strand[len(strand)-1].CachedOut()

	if firstCachedIn == nil || lastCachedOut == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	// Calculate totals
	var offersUsed uint32
	inactive := false
	for _, step := range strand {
		offersUsed += step.OffersUsed()
		if step.Inactive() {
			inactive = true
		}
	}

	return StrandResult{
		Success:    true,
		In:         *firstCachedIn,
		Out:        *lastCachedOut,
		Sandbox:    sb,
		OffsToRm:   ofrsToRm,
		OffersUsed: offersUsed,
		Inactive:   inactive,
	}
}

// executeWithMaxIn executes when we have a maximum input constraint
func executeWithMaxIn(
	baseView *PaymentSandbox,
	strand Strand,
	maxIn EitherAmount,
	ofrsToRm map[[32]byte]bool,
) StrandResult {
	// Create fresh sandbox
	sb := NewChildSandbox(baseView)

	// Start forward pass from the beginning with maxIn
	in := maxIn

	for i := 0; i < len(strand); i++ {
		step := strand[i]

		// Execute forward
		actualIn, actualOut := step.Fwd(sb, baseView, ofrsToRm, in)

		// Check if step consumed all input
		if step.IsZero(actualOut) && !step.IsZero(in) {
			return StrandResult{
				Success:  false,
				In:       ZeroXRPEitherAmount(),
				Out:      ZeroXRPEitherAmount(),
				Sandbox:  nil,
				OffsToRm: ofrsToRm,
				Inactive: true,
			}
		}

		// For the first step, record actual input consumed
		if i == 0 {
			// Track that we might not use all of maxIn
			_ = actualIn
		}

		// Output becomes input for next step
		in = actualOut
	}

	// Get final results
	firstCachedIn := strand[0].CachedIn()
	lastCachedOut := strand[len(strand)-1].CachedOut()

	if firstCachedIn == nil || lastCachedOut == nil {
		return StrandResult{
			Success:  false,
			In:       ZeroXRPEitherAmount(),
			Out:      ZeroXRPEitherAmount(),
			Sandbox:  nil,
			OffsToRm: ofrsToRm,
			Inactive: true,
		}
	}

	// Calculate totals
	var offersUsed uint32
	inactive := false
	for _, step := range strand {
		offersUsed += step.OffersUsed()
		if step.Inactive() {
			inactive = true
		}
	}

	return StrandResult{
		Success:    true,
		In:         *firstCachedIn,
		Out:        *lastCachedOut,
		Sandbox:    sb,
		OffsToRm:   ofrsToRm,
		OffersUsed: offersUsed,
		Inactive:   inactive,
	}
}

// ExecuteStrandReverse is a helper that only executes the reverse pass
// Useful for quality estimation
func ExecuteStrandReverse(
	view *PaymentSandbox,
	strand Strand,
	requestedOut EitherAmount,
) (EitherAmount, EitherAmount) {
	if len(strand) == 0 {
		return ZeroXRPEitherAmount(), ZeroXRPEitherAmount()
	}

	// Create sandbox for execution
	sb := NewChildSandbox(view)
	ofrsToRm := make(map[[32]byte]bool)

	// Work backwards
	out := requestedOut
	for i := len(strand) - 1; i >= 0; i-- {
		step := strand[i]
		actualIn, _ := step.Rev(sb, view, ofrsToRm, out)
		out = actualIn
	}

	// Return first step input and last step output
	firstCachedIn := strand[0].CachedIn()
	lastCachedOut := strand[len(strand)-1].CachedOut()

	if firstCachedIn == nil || lastCachedOut == nil {
		return ZeroXRPEitherAmount(), ZeroXRPEitherAmount()
	}

	return *firstCachedIn, *lastCachedOut
}

// StrandIterator allows iterating over strands for the flow algorithm
type StrandIterator struct {
	strands     []Strand
	results     []StrandResult
	activeIndex int
}

// NewStrandIterator creates an iterator over strands
func NewStrandIterator(strands []Strand) *StrandIterator {
	return &StrandIterator{
		strands:     strands,
		results:     make([]StrandResult, len(strands)),
		activeIndex: 0,
	}
}

// HasNext returns true if there are more strands to process
func (it *StrandIterator) HasNext() bool {
	return it.activeIndex < len(it.strands)
}

// Next returns the next strand and advances the iterator
func (it *StrandIterator) Next() (Strand, int) {
	if it.activeIndex >= len(it.strands) {
		return nil, -1
	}
	strand := it.strands[it.activeIndex]
	index := it.activeIndex
	it.activeIndex++
	return strand, index
}

// SetResult stores the result for a strand
func (it *StrandIterator) SetResult(index int, result StrandResult) {
	if index >= 0 && index < len(it.results) {
		it.results[index] = result
	}
}

// GetResult retrieves the result for a strand
func (it *StrandIterator) GetResult(index int) StrandResult {
	if index >= 0 && index < len(it.results) {
		return it.results[index]
	}
	return StrandResult{}
}

// Reset resets the iterator to the beginning
func (it *StrandIterator) Reset() {
	it.activeIndex = 0
}
