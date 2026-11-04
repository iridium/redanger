package com.redanger.classof09;

/** A shown image (or movie) with Ren'Py transform properties. */
final class Sprite {
	static final int XALIGN = 0, YALIGN = 1, ALPHA = 2, ZOOM = 3, XZOOM = 4, ROTATE = 5;

	final int tag;
	int imgId;
	Img img;
	Video video;
	/** xalign/yalign/alpha/zoom/xzoom are x1000, rotate is degrees x10. */
	final int[] prop = { 500, 1000, 1000, 1000, 1000, 0 };
	Atl atl;

	Sprite(int tag) {
		this.tag = tag;
	}
}
