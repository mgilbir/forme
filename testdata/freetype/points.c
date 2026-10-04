/*
 * Writes every glyph's outline points as FreeType loads them, unscaled and
 * unhinted (FT_LOAD_NO_SCALE | FT_LOAD_NO_HINTING), for points.py to record:
 *
 *   points FONT [AXIS=VALUE ...]
 *
 * One line a glyph: "G gid" and then each point as x,y,tag in FreeType's
 * order, tag 1 for a point on the curve, 0 for a quadratic control point and
 * 2 for a cubic one; or "G gid none" for a glyph FreeType loads no outline
 * for. Axis values are in design units, set with FT_Set_Var_Design_Coordinates.
 * The first line is the FreeType version.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_MULTIPLE_MASTERS_H

int main(int argc, char **argv) {
	FT_Library lib;
	FT_Face face;
	if (argc < 2 || FT_Init_FreeType(&lib) || FT_New_Face(lib, argv[1], 0, &face)) {
		fprintf(stderr, "usage: points FONT [AXIS=VALUE ...]\n");
		return 1;
	}
	FT_Int major, minor, patch;
	FT_Library_Version(lib, &major, &minor, &patch);
	printf("freetype %d.%d.%d\n", major, minor, patch);
	if (argc > 2) {
		FT_MM_Var *mm;
		if (FT_Get_MM_Var(face, &mm)) {
			fprintf(stderr, "%s has no design space\n", argv[1]);
			return 1;
		}
		FT_Fixed *coords = calloc(mm->num_axis, sizeof(FT_Fixed));
		for (FT_UInt a = 0; a < mm->num_axis; a++)
			coords[a] = mm->axis[a].def;
		for (int i = 2; i < argc; i++) {
			char tag[5] = {0};
			double v;
			if (sscanf(argv[i], "%4[^=]=%lf", tag, &v) != 2)
				return 1;
			for (FT_UInt a = 0; a < mm->num_axis; a++) {
				FT_ULong t = mm->axis[a].tag;
				char at[5] = {(char)(t >> 24), (char)(t >> 16), (char)(t >> 8), (char)t, 0};
				if (!strcmp(at, tag))
					coords[a] = (FT_Fixed)(v * 65536.0);
			}
		}
		if (FT_Set_Var_Design_Coordinates(face, mm->num_axis, coords))
			return 1;
	}
	for (FT_Long gid = 0; gid < face->num_glyphs; gid++) {
		if (FT_Load_Glyph(face, (FT_UInt)gid, FT_LOAD_NO_SCALE | FT_LOAD_NO_HINTING) ||
		    face->glyph->format != FT_GLYPH_FORMAT_OUTLINE) {
			printf("G %ld none\n", gid);
			continue;
		}
		FT_Outline *o = &face->glyph->outline;
		printf("G %ld", gid);
		for (int p = 0; p < o->n_points; p++)
			printf(" %ld,%ld,%d", o->points[p].x, o->points[p].y, FT_CURVE_TAG(o->tags[p]));
		printf("\n");
	}
	return 0;
}
