package betterleaks

import blsources "github.com/betterleaks/betterleaks/sources"

const (
	AttrGitAuthorEmail = blsources.AttrGitAuthorEmail
	AttrGitAuthorName  = blsources.AttrGitAuthorName
	AttrGitDate        = blsources.AttrGitDate
	AttrGitMessage     = blsources.AttrGitMessage
	AttrGitSHA         = blsources.AttrGitSHA
	AttrPath           = blsources.AttrPath
	AttrURL            = blsources.AttrURL

	// Custom ones defined by this module
	AttrOCIImageDigest          = "oci.image.digest"
	AttrOCIImageAuthorName      = "oci.image.author_name"
	AttrOCIImageAuthorEmail     = "oci.image.author_email"
	AttrOCIImageMaintainerName  = "oci.image.maintainer_name"
	AttrOCIImageMaintainerEmail = "oci.image.maintainer_email"
)
