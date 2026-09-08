# Comparison fixtures

PNG fixtures copied from github.com/raf555/pixelmatch v1.0.1,
internal/testutil/data. Its compatibility suite attributes these images to
mapbox/pixelmatch. The port and original Mapbox ISC licenses are retained here.

At threshold 0.1, 1a.png versus 1b.png has 106 mismatched pixels and produces
1diffdefaultthreshold.png. At threshold 0.05 it has 143 mismatched pixels.
These fixed expectations guard dependency upgrades; they do not assert parity
with every version of JavaScript Pixelmatch.
