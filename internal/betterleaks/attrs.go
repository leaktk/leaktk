package betterleaks

import blsources "github.com/betterleaks/betterleaks/sources"

const (
	AttrGitSHA         = blsources.AttrGitSHA
	AttrGitAuthorName  = blsources.AttrGitAuthorName
	AttrGitAuthorEmail = blsources.AttrGitAuthorEmail
	AttrGitDate        = blsources.AttrGitDate
	AttrGitMessage     = blsources.AttrGitMessage
	AttrPath           = blsources.AttrPath
	AttrURL            = blsources.AttrURL

	// Custom ones defined by this module
	AttrOCIImageDigest          = "oci.image.digest"
	AttrOCIImageAuthorName      = "oci.image.author_name"
	AttrOCIImageAuthorEmail     = "oci.image.author_email"
	AttrOCIImageMaintainerName  = "oci.image.maintainer_name"
	AttrOCIImageMaintainerEmail = "oci.image.maintainer_email"
)
