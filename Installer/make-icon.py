"""Builds Installer/SemiBase.ico from logo.png: the pictogram without its caption, the white
background made transparent. Run from the repository root: python Installer/make-icon.py"""
from PIL import Image

source, target = "logo.png", "Installer/SemiBase.ico"
image = Image.open(source).convert("RGB")
# The pictogram sits above the "semibase" caption.
image = image.crop((0, 0, image.width, 845))
pixels = image.load()
out = Image.new("RGBA", image.size)
dst = out.load()
for y in range(image.height):
    for x in range(image.width):
        r, g, b = pixels[x, y]
        alpha = (255 - min(r, g, b)) / 255
        if alpha < 0.04:
            dst[x, y] = (0, 0, 0, 0)
            continue
        alpha = min(1.0, alpha * 1.05)
        # Undo the blend with white so anti-aliased edges keep their colour.
        dst[x, y] = tuple(max(0, min(255, round((c - 255 * (1 - alpha)) / alpha))) for c in (r, g, b)) + (round(alpha * 255),)
box = out.getbbox()
out = out.crop(box)
side = max(out.size)
margin = round(side * 0.03)
square = Image.new("RGBA", (side + 2 * margin, side + 2 * margin), (0, 0, 0, 0))
square.paste(out, ((square.width - out.width) // 2, (square.height - out.height) // 2))
square = square.resize((256, 256), Image.LANCZOS)
square.save(target, sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
