/*
 * Writes every glyph of every bitmap strike as FreeType loads it, for
 * strikes.py to record:
 *
 *   strikes FONT
 *
 * For each strike, in the order the font states them, a line "S index ppemX
 * ppemY", where ppemX and ppemY are FreeType's x_ppem and y_ppem rounded to
 * whole pixels; and then for each glyph a line "G gid depth width rows left
 * top" and the glyph's samples, row by row from the top, each from 0 to
 * 2^depth-1, written in hexadecimal, a row's samples separated by commas; or
 * "G gid none" for a glyph FreeType loads no bitmap of. The strike is selected
 * with FT_Select_Size and each glyph loaded with FT_LOAD_DEFAULT, which for a
 * face with no outlines is its bitmap. The first line is the FreeType version.
 */
#include <stdio.h>
#include <stdlib.h>
#include <ft2build.h>
#include FT_FREETYPE_H

static int depthOf(unsigned char mode) {
	switch (mode) {
	case FT_PIXEL_MODE_MONO:
		return 1;
	case FT_PIXEL_MODE_GRAY2:
		return 2;
	case FT_PIXEL_MODE_GRAY4:
		return 4;
	case FT_PIXEL_MODE_GRAY:
		return 8;
	}
	return 0;
}

int main(int argc, char **argv) {
	FT_Library lib;
	FT_Face face;
	if (argc != 2 || FT_Init_FreeType(&lib) || FT_New_Face(lib, argv[1], 0, &face)) {
		fprintf(stderr, "usage: strikes FONT\n");
		return 1;
	}
	FT_Int major, minor, patch;
	FT_Library_Version(lib, &major, &minor, &patch);
	printf("freetype %d.%d.%d\n", major, minor, patch);
	for (int s = 0; s < face->num_fixed_sizes; s++) {
		if (FT_Select_Size(face, s)) {
			fprintf(stderr, "strike %d cannot be selected\n", s);
			return 1;
		}
		FT_Bitmap_Size *size = &face->available_sizes[s];
		printf("S %d %ld %ld\n", s, (long)((size->x_ppem + 32) >> 6), (long)((size->y_ppem + 32) >> 6));
		for (FT_Long gid = 0; gid < face->num_glyphs; gid++) {
			if (FT_Load_Glyph(face, (FT_UInt)gid, FT_LOAD_DEFAULT) ||
			    face->glyph->format != FT_GLYPH_FORMAT_BITMAP) {
				printf("G %ld none\n", gid);
				continue;
			}
			FT_Bitmap *b = &face->glyph->bitmap;
			int depth = depthOf(b->pixel_mode);
			if (depth == 0) {
				fprintf(stderr, "glyph %ld: pixel mode %d\n", gid, b->pixel_mode);
				return 1;
			}
			printf("G %ld %d %u %u %d %d", gid, depth, b->width, b->rows,
			       face->glyph->bitmap_left, face->glyph->bitmap_top);
			for (unsigned y = 0; y < b->rows; y++) {
				const unsigned char *row = b->buffer + (long)y * b->pitch;
				printf(" ");
				for (unsigned x = 0; x < b->width; x++) {
					unsigned bit = x * depth;
					unsigned v = (row[bit / 8] >> (8 - depth - bit % 8)) & ((1u << depth) - 1);
					printf("%s%x", x ? "," : "", v);
				}
			}
			printf("\n");
		}
	}
	FT_Done_Face(face);
	FT_Done_FreeType(lib);
	return 0;
}
