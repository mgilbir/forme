package html

// The names HTML's parser gives SVG and MathML back.
//
// HTML folds every tag and attribute name it reads to ASCII lower case, and
// SVG's names are not all lower case: viewBox, preserveAspectRatio,
// foreignObject, linearGradient. So when the tree builder meets a start tag in
// foreign content it puts the name back through a table, §13.2.6.5's "adjust
// SVG tag names" for an SVG element and §13.2.6.1's "adjust SVG attributes" and
// "adjust MathML attributes" for the attributes, and what comes out is the name
// SVG and MathML define. That is why "<svg VIEWBOX='0 0 1 1'>" in an HTML
// document is an svg with a viewBox, and why nothing past the table is folded:
// after it the names are case-sensitive, as they are in XML. An SVG file, and
// an <svg> in an XHTML document, is XML, never goes through the table, and
// means what it spells: "<svg VIEWBOX='0 0 1 1'>" there has no viewBox.
//
// The tables are the standard's, entry for entry, and TestTheForeignNameTables
// holds them to the shape the standard gives them.

// AdjustSVGTagName is §13.2.6.5's table: the SVG element name for a tag name
// HTML has folded to lower case, or the name unchanged when SVG spells it that
// way.
func AdjustSVGTagName(lower string) string {
	if n, ok := svgTagNames[lower]; ok {
		return n
	}
	return lower
}

// AdjustSVGAttributeName is §13.2.6.1's "adjust SVG attributes": the SVG
// attribute name for one HTML has folded to lower case, or the name unchanged.
func AdjustSVGAttributeName(lower string) string {
	if n, ok := svgAttributeNames[lower]; ok {
		return n
	}
	return lower
}

// AdjustMathMLAttributeName is §13.2.6.1's "adjust MathML attributes", whose
// table has one entry.
func AdjustMathMLAttributeName(lower string) string {
	if lower == "definitionurl" {
		return "definitionURL"
	}
	return lower
}

var svgTagNames = map[string]string{
	"altglyph":            "altGlyph",
	"altglyphdef":         "altGlyphDef",
	"altglyphitem":        "altGlyphItem",
	"animatecolor":        "animateColor",
	"animatemotion":       "animateMotion",
	"animatetransform":    "animateTransform",
	"clippath":            "clipPath",
	"feblend":             "feBlend",
	"fecolormatrix":       "feColorMatrix",
	"fecomponenttransfer": "feComponentTransfer",
	"fecomposite":         "feComposite",
	"feconvolvematrix":    "feConvolveMatrix",
	"fediffuselighting":   "feDiffuseLighting",
	"fedisplacementmap":   "feDisplacementMap",
	"fedistantlight":      "feDistantLight",
	"fedropshadow":        "feDropShadow",
	"feflood":             "feFlood",
	"fefunca":             "feFuncA",
	"fefuncb":             "feFuncB",
	"fefuncg":             "feFuncG",
	"fefuncr":             "feFuncR",
	"fegaussianblur":      "feGaussianBlur",
	"feimage":             "feImage",
	"femerge":             "feMerge",
	"femergenode":         "feMergeNode",
	"femorphology":        "feMorphology",
	"feoffset":            "feOffset",
	"fepointlight":        "fePointLight",
	"fespecularlighting":  "feSpecularLighting",
	"fespotlight":         "feSpotLight",
	"fetile":              "feTile",
	"feturbulence":        "feTurbulence",
	"foreignobject":       "foreignObject",
	"glyphref":            "glyphRef",
	"lineargradient":      "linearGradient",
	"radialgradient":      "radialGradient",
	"textpath":            "textPath",
}

var svgAttributeNames = map[string]string{
	"attributename":       "attributeName",
	"attributetype":       "attributeType",
	"basefrequency":       "baseFrequency",
	"baseprofile":         "baseProfile",
	"calcmode":            "calcMode",
	"clippathunits":       "clipPathUnits",
	"diffuseconstant":     "diffuseConstant",
	"edgemode":            "edgeMode",
	"filterunits":         "filterUnits",
	"glyphref":            "glyphRef",
	"gradienttransform":   "gradientTransform",
	"gradientunits":       "gradientUnits",
	"kernelmatrix":        "kernelMatrix",
	"kernelunitlength":    "kernelUnitLength",
	"keypoints":           "keyPoints",
	"keysplines":          "keySplines",
	"keytimes":            "keyTimes",
	"lengthadjust":        "lengthAdjust",
	"limitingconeangle":   "limitingConeAngle",
	"markerheight":        "markerHeight",
	"markerunits":         "markerUnits",
	"markerwidth":         "markerWidth",
	"maskcontentunits":    "maskContentUnits",
	"maskunits":           "maskUnits",
	"numoctaves":          "numOctaves",
	"pathlength":          "pathLength",
	"patterncontentunits": "patternContentUnits",
	"patterntransform":    "patternTransform",
	"patternunits":        "patternUnits",
	"pointsatx":           "pointsAtX",
	"pointsaty":           "pointsAtY",
	"pointsatz":           "pointsAtZ",
	"preservealpha":       "preserveAlpha",
	"preserveaspectratio": "preserveAspectRatio",
	"primitiveunits":      "primitiveUnits",
	"refx":                "refX",
	"refy":                "refY",
	"repeatcount":         "repeatCount",
	"repeatdur":           "repeatDur",
	"requiredextensions":  "requiredExtensions",
	"requiredfeatures":    "requiredFeatures",
	"specularconstant":    "specularConstant",
	"specularexponent":    "specularExponent",
	"spreadmethod":        "spreadMethod",
	"startoffset":         "startOffset",
	"stddeviation":        "stdDeviation",
	"stitchtiles":         "stitchTiles",
	"surfacescale":        "surfaceScale",
	"systemlanguage":      "systemLanguage",
	"tablevalues":         "tableValues",
	"targetx":             "targetX",
	"targety":             "targetY",
	"textlength":          "textLength",
	"viewbox":             "viewBox",
	"viewtarget":          "viewTarget",
	"xchannelselector":    "xChannelSelector",
	"ychannelselector":    "yChannelSelector",
	"zoomandpan":          "zoomAndPan",
}
