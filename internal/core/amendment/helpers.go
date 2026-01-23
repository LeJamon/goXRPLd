// Copyright (c) 2024-2025. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amendment

// AMMFullyEnabled returns true if all AMM-related amendments are enabled.
// AMM requires both featureAMM and fixUniversalNumber to function.
func (r *Rules) AMMFullyEnabled() bool {
	return r.Enabled(FeatureAMM) && r.Enabled(FeatureFixUniversalNumber)
}

// AMMWithFixesEnabled returns true if AMM with all fixes is enabled.
func (r *Rules) AMMWithFixesEnabled() bool {
	return r.AMMFullyEnabled() &&
		r.Enabled(FeatureFixAMMv1_1) &&
		r.Enabled(FeatureFixAMMv1_2) &&
		r.Enabled(FeatureFixAMMv1_3)
}

// PermissionedDomainsFullyEnabled returns true if permissioned domains are fully enabled.
// Requires both PermissionedDomains and Credentials amendments.
func (r *Rules) PermissionedDomainsFullyEnabled() bool {
	return r.Enabled(FeaturePermissionedDomains) && r.Enabled(FeatureCredentials)
}

// NFTsWithDynamicEnabled returns true if NFTs with dynamic features are enabled.
func (r *Rules) NFTsWithDynamicEnabled() bool {
	return r.NFTsEnabled() && r.Enabled(FeatureDynamicNFT)
}

// VaultsEnabled returns true if the SingleAssetVault amendment is enabled.
func (r *Rules) VaultsEnabled() bool {
	return r.Enabled(FeatureSingleAssetVault)
}

// PermissionDelegationEnabled returns true if the PermissionDelegation amendment is enabled.
func (r *Rules) PermissionDelegationEnabled() bool {
	return r.Enabled(FeaturePermissionDelegation)
}
