import argparse
import math
import os
import sys
from dataclasses import dataclass
from multiprocessing import Pool

import cv2
import numpy as np
from PIL import Image

ROOT = os.path.dirname(os.path.abspath(__file__))
ASSETS = os.path.join(ROOT, "assets")

RANKS = ["A", "K", "Q", "J", "10", "9", "8", "7"]
SUITS = ["S", "H", "D", "C"]
CARDS = [f"{rank}{suit}" for suit in SUITS for rank in RANKS]
CLASS_INDEX = {card: index for index, card in enumerate(CARDS)}

RANK_WORDS = {"A": "ace", "K": "king", "Q": "queen", "J": "jack"}
SUIT_WORDS = {"S": "spades", "H": "hearts", "D": "diamonds", "C": "clubs"}
SUIT_SINGULAR = {"S": "spade", "H": "heart", "D": "diamond", "C": "club"}

SPRITE_WIDTH = 520
MIN_VISIBILITY = 0.5
MIN_BOX_SIZE = 12


@dataclass
class Deck:
    name: str
    corner: tuple[float, float, float, float]
    outlined: bool

    def path(self, card: str) -> str:
        raise NotImplementedError


class AustinDeck(Deck):
    def path(self, card):
        rank, suit = card[:-1], card[-1]
        word = RANK_WORDS.get(rank, rank)
        return os.path.join(
            ASSETS,
            "austin/Public-Domain-and-CC0-Playing-Cards-main/png cards/card fronts",
            SUIT_WORDS[suit],
            f"{word} of {SUIT_WORDS[suit]}.png",
        )


class SaulDeck(Deck):
    def __init__(self, name, variant, corner, outlined):
        super().__init__(name, corner, outlined)
        self.variant = variant

    def path(self, card):
        rank, suit = card[:-1], card[-1]
        word = RANK_WORDS.get(rank, rank).capitalize() if rank in RANK_WORDS else rank
        return os.path.join(
            ASSETS, "saul_png", self.variant, f"{SUIT_SINGULAR[suit]}{word}.png"
        )


class SeeligDeck(Deck):
    def __init__(self, name, font, corner, outlined):
        super().__init__(name, corner, outlined)
        self.font = font

    def path(self, card):
        rank, suit = card[:-1], card[-1]
        word = RANK_WORDS.get(rank, rank)
        return os.path.join(
            ASSETS, "seelig_png", self.font, f"{word}_of_{SUIT_WORDS[suit]}.png"
        )


DECKS = [
    AustinDeck("austin", (0.03, 0.02, 0.16, 0.225), outlined=False),
    SaulDeck("saul2", "Vertical2", (0.02, 0.025, 0.215, 0.30), outlined=True),
    SaulDeck("saul4", "Vertical4", (0.02, 0.025, 0.215, 0.30), outlined=True),
    SeeligDeck("seelig_abril", "abril", (0.015, 0.012, 0.175, 0.245), outlined=False),
    SeeligDeck("seelig_roboto", "roboto", (0.015, 0.012, 0.175, 0.245), outlined=False),
]

_sprites: dict[tuple[str, str], np.ndarray] = {}


def load_sprite(deck: Deck, card: str) -> np.ndarray:
    key = (deck.name, card)
    if key not in _sprites:
        image = Image.open(deck.path(card)).convert("RGBA")
        height = round(image.height * SPRITE_WIDTH / image.width)
        image = image.resize((SPRITE_WIDTH, height), Image.LANCZOS)
        _sprites[key] = np.array(image)
    return _sprites[key]


def make_background(rng: np.random.Generator, width: int, height: int) -> np.ndarray:
    kind = rng.choice(["solid", "gradient", "noise", "wood", "stripes", "dark"])
    base = rng.uniform(40, 235, 3)
    if rng.random() < 0.55:
        base = rng.uniform(60, 235) + rng.uniform(-18, 18, 3)
    if kind == "dark":
        base = rng.uniform(15, 80, 3)

    image = np.ones((height, width, 3), np.float32) * base

    if kind == "gradient":
        angle = rng.uniform(0, 2 * math.pi)
        ys, xs = np.mgrid[0:height, 0:width]
        ramp = (xs * math.cos(angle) + ys * math.sin(angle)) / max(width, height)
        image += ramp[..., None] * rng.uniform(-90, 90, 3)
    elif kind in ("noise", "wood"):
        low = rng.normal(0, 1, (height // 40 + 2, width // 40 + 2, 3)).astype(
            np.float32
        )
        image += cv2.resize(
            low, (width, height), interpolation=cv2.INTER_CUBIC
        ) * rng.uniform(10, 35)
        if kind == "wood":
            grain = rng.normal(0, 1, (height // 2, 8, 1)).astype(np.float32)
            image += cv2.resize(grain, (width, height), interpolation=cv2.INTER_CUBIC)[
                ..., None
            ] * rng.uniform(6, 16)
    elif kind == "stripes":
        period = rng.uniform(30, 160)
        ys = np.arange(height)[:, None, None]
        image += (np.sin(ys / period * 2 * math.pi) * rng.uniform(5, 30)) * np.ones(
            (1, width, 3), np.float32
        )

    for _ in range(rng.integers(0, 7)):
        color = rng.uniform(0, 255, 3)
        cx, cy = rng.uniform(0, width), rng.uniform(0, height)
        axes = (int(rng.uniform(20, 260)), int(rng.uniform(20, 200)))
        cv2.ellipse(
            image,
            (int(cx), int(cy)),
            axes,
            rng.uniform(0, 180),
            0,
            360,
            color.tolist(),
            -1,
        )

    for _ in range(rng.integers(0, 4)):
        color = rng.uniform(0, 255, 3)
        x0, y0 = rng.uniform(0, width), rng.uniform(0, height)
        x1, y1 = x0 + rng.uniform(40, 500), y0 + rng.uniform(20, 160)
        cv2.rectangle(image, (int(x0), int(y0)), (int(x1), int(y1)), color.tolist(), -1)

    return np.clip(image, 0, 255).astype(np.uint8)


def distress(sprite: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    result = sprite.copy()
    probability = rng.uniform(0.002, 0.03)
    speck = rng.random(result.shape[:2]) < probability
    speck = cv2.dilate(speck.astype(np.uint8), np.ones((2, 2), np.uint8)).astype(bool)
    strength = rng.uniform(0.5, 0.95)
    result[..., :3][speck] = (
        result[..., :3][speck] * (1 - strength) + 255 * strength
    ).astype(np.uint8)
    return result


def outline(sprite: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    result = sprite.copy()
    alpha = (result[..., 3] > 128).astype(np.uint8)
    thickness = max(1, round(SPRITE_WIDTH / rng.uniform(120, 260)))
    inner = cv2.erode(alpha, np.ones((thickness * 2 + 1, thickness * 2 + 1), np.uint8))
    edge = (alpha > 0) & (inner == 0)
    shade = rng.uniform(40, 160)
    result[..., :3][edge] = shade
    return result


def prepare_sprite(
    deck: Deck, card: str, target_width: int, rng: np.random.Generator
) -> np.ndarray:
    sprite = load_sprite(deck, card)
    if rng.random() < 0.5:
        sprite = distress(sprite, rng)
    if not deck.outlined or rng.random() < 0.3:
        sprite = outline(sprite, rng)

    height = round(sprite.shape[0] * target_width / sprite.shape[1])
    resized = cv2.resize(sprite, (target_width, height), interpolation=cv2.INTER_AREA)

    alpha = resized[..., 3:4].astype(np.float32) / 255
    bgr = resized[..., 2::-1].astype(np.float32)
    premultiplied = np.concatenate([bgr * alpha, alpha * 255], axis=2)
    return premultiplied


def translation(tx: float, ty: float) -> np.ndarray:
    return np.array([[1, 0, tx], [0, 1, ty], [0, 0, 1]], np.float64)


def rotation(degrees: float) -> np.ndarray:
    angle = math.radians(degrees)
    cos, sin = math.cos(angle), math.sin(angle)
    return np.array([[cos, -sin, 0], [sin, cos, 0], [0, 0, 1]], np.float64)


def scaling(factor: float) -> np.ndarray:
    return np.array([[factor, 0, 0], [0, factor, 0], [0, 0, 1]], np.float64)


@dataclass
class Placement:
    card: str
    matrix: np.ndarray
    sprite_size: tuple[int, int]


def fan_layout(rng, count, card_width, card_height, center):
    spread = float(min(rng.uniform(5, 11) * count, 115))
    pivot_y = rng.uniform(0.85, 1.3) * card_height
    pivot = np.array([card_width / 2, pivot_y])
    angles = (
        np.linspace(-spread / 2, spread / 2, count) if count > 1 else np.array([0.0])
    )
    if rng.random() < 0.3:
        angles = angles[::-1]

    matrices = []
    for angle in angles:
        matrices.append(translation(*center) @ rotation(angle) @ translation(*(-pivot)))
    return matrices


def stair_layout(rng, count, card_width, card_height, width, height):
    matrices = []
    remaining = count
    groups = int(rng.integers(2, 5))
    for group in range(groups):
        size = (
            remaining
            if group == groups - 1
            else int(rng.integers(1, max(2, remaining - (groups - group - 1) + 1)))
        )
        size = max(1, min(size, remaining))
        remaining -= size

        origin = np.array(
            [rng.uniform(0.15, 0.85) * width, rng.uniform(0.15, 0.85) * height]
        )
        direction = rng.uniform(0, 2 * math.pi)
        step = rng.uniform(0.18, 0.4) * card_width
        tilt = rng.uniform(-12, 12)
        for index in range(size):
            offset = origin + index * step * np.array(
                [math.cos(direction), math.sin(direction)]
            )
            jitter = rng.uniform(-3, 3)
            matrices.append(
                translation(*offset)
                @ rotation(tilt + jitter)
                @ translation(-card_width / 2, -card_height / 2)
            )
        if remaining <= 0:
            break
    return matrices


def scatter_layout(rng, count, card_width, card_height, width, height):
    matrices = []
    for _ in range(count):
        center = np.array(
            [rng.uniform(0.1, 0.9) * width, rng.uniform(0.1, 0.9) * height]
        )
        matrices.append(
            translation(*center)
            @ rotation(rng.uniform(0, 360))
            @ translation(-card_width / 2, -card_height / 2)
        )
    return matrices


def global_transform(rng, width, height):
    if rng.random() < 0.5:
        angle = rng.integers(0, 4) * 90 + rng.uniform(-15, 15)
    else:
        angle = rng.uniform(0, 360)
    factor = rng.uniform(0.75, 1.1)
    center = (width / 2, height / 2)
    transform = (
        translation(*center)
        @ rotation(angle)
        @ scaling(factor)
        @ translation(-center[0], -center[1])
    )

    source = np.float32([[0, 0], [width, 0], [width, height], [0, height]])
    jitter = rng.uniform(-0.06, 0.06, (4, 2)) * np.array([width, height])
    perspective = cv2.getPerspectiveTransform(
        source, (source + jitter).astype(np.float32)
    )
    return perspective @ transform


def draw_hand(image, idmask, rng, anchor, radius):
    skin = np.array(
        [rng.uniform(190, 245), rng.uniform(140, 200), rng.uniform(110, 170)]
    )
    skin = skin * rng.uniform(0.55, 1.0)
    layer = np.zeros(image.shape[:2], np.uint8)
    for _ in range(rng.integers(2, 5)):
        offset = rng.normal(0, radius * 0.3, 2)
        axes = (
            int(radius * rng.uniform(0.5, 1.0)),
            int(radius * rng.uniform(0.35, 0.7)),
        )
        center = (int(anchor[0] + offset[0]), int(anchor[1] + offset[1]))
        cv2.ellipse(layer, center, axes, rng.uniform(0, 180), 0, 360, 255, -1)

    soft = cv2.GaussianBlur(layer, (0, 0), 2.0).astype(np.float32)[..., None] / 255
    image[:] = (image * (1 - soft) + skin[::-1] * soft).astype(np.uint8)
    idmask[layer > 0] = 255


def render_scene(rng, decks):
    width, height = (1280, 960) if rng.random() < 0.5 else (960, 1280)
    image = make_background(rng, width, height)
    idmask = np.zeros((height, width), np.uint8)

    mode = rng.choice(["fan", "stairs", "scatter", "empty"], p=[0.45, 0.33, 0.17, 0.05])
    if mode == "empty":
        return image, idmask, []

    count = int(rng.integers(2, 13)) if mode != "scatter" else int(rng.integers(1, 9))
    cards = list(rng.choice(CARDS, size=count, replace=False))
    deck = decks[int(rng.integers(len(decks)))]

    card_width = int(rng.uniform(210, 360))
    sprite_ratio = load_sprite(deck, cards[0]).shape[0] / SPRITE_WIDTH
    card_height = int(card_width * sprite_ratio)

    if mode == "fan":
        center = (rng.uniform(0.35, 0.65) * width, rng.uniform(0.55, 0.85) * height)
        matrices = fan_layout(rng, count, card_width, card_height, center)
    elif mode == "stairs":
        matrices = stair_layout(rng, count, card_width, card_height, width, height)
    else:
        matrices = scatter_layout(rng, count, card_width, card_height, width, height)

    global_matrix = global_transform(rng, width, height)
    placements = []
    for card, matrix in zip(cards, matrices, strict=False):
        sprite = prepare_sprite(deck, card, card_width, rng)
        placements.append((card, global_matrix @ matrix, sprite))

    image_f = image.astype(np.float32)
    for index, (card, matrix, sprite) in enumerate(placements, start=1):
        shadow_shift = translation(rng.uniform(2, 9), rng.uniform(3, 12))
        warped = cv2.warpPerspective(
            sprite, shadow_shift @ matrix, (width, height), flags=cv2.INTER_LINEAR
        )
        shadow = (
            cv2.GaussianBlur(warped[..., 3], (0, 0), rng.uniform(2, 6))
            / 255
            * rng.uniform(0.2, 0.5)
        )
        image_f *= 1 - shadow[..., None]

        warped = cv2.warpPerspective(
            sprite, matrix, (width, height), flags=cv2.INTER_LINEAR
        )
        alpha = warped[..., 3:4] / 255
        image_f = warped[..., :3] + image_f * (1 - alpha)
        idmask[warped[..., 3] > 128] = index

    image = np.clip(image_f, 0, 255).astype(np.uint8)

    if mode == "fan" and rng.random() < 0.6:
        pivot = global_matrix @ np.array([*center, 1.0])
        draw_hand(
            image, idmask, rng, pivot[:2] / pivot[2], card_width * rng.uniform(0.3, 0.6)
        )
    elif rng.random() < 0.15:
        draw_hand(
            image,
            idmask,
            rng,
            (rng.uniform(0, width), rng.uniform(0, height)),
            rng.uniform(60, 200),
        )

    labels = collect_labels(placements, deck, idmask, width, height)
    return image, idmask, labels


def polygon_area(points):
    x, y = points[:, 0], points[:, 1]
    return 0.5 * abs(np.dot(x, np.roll(y, 1)) - np.dot(y, np.roll(x, 1)))


def collect_labels(placements, deck, idmask, width, height):
    labels = []
    x0, y0, x1, y1 = deck.corner

    for index, (card, matrix, sprite) in enumerate(placements, start=1):
        sprite_height, sprite_width = sprite.shape[:2]
        boxes = [
            (x0, y0, x1, y1),
            (1 - x1, 1 - y1, 1 - x0, 1 - y0),
        ]

        for bx0, by0, bx1, by1 in boxes:
            corners = np.float32(
                [
                    [bx0 * sprite_width, by0 * sprite_height],
                    [bx1 * sprite_width, by0 * sprite_height],
                    [bx1 * sprite_width, by1 * sprite_height],
                    [bx0 * sprite_width, by1 * sprite_height],
                ]
            )
            projected = cv2.perspectiveTransform(corners[None], matrix)[0]

            full_area = polygon_area(projected)
            mask = np.zeros((height, width), np.uint8)
            cv2.fillConvexPoly(mask, np.round(projected).astype(np.int32), 1)
            inside_area = int(mask.sum())
            if full_area < 150 or inside_area < 0.85 * full_area:
                continue

            visible = (idmask == index) & (mask > 0)
            if visible.sum() < MIN_VISIBILITY * inside_area:
                continue

            ys, xs = np.where(visible)
            left, right, top, bottom = xs.min(), xs.max() + 1, ys.min(), ys.max() + 1
            if right - left < MIN_BOX_SIZE or bottom - top < MIN_BOX_SIZE:
                continue

            labels.append((CLASS_INDEX[card], left, top, right, bottom))

    return labels


def photometric(image, rng):
    result = image.astype(np.float32)

    result = result * rng.uniform(0.6, 1.3) + rng.uniform(-30, 30)
    result *= rng.uniform(0.85, 1.15, 3)
    result = 255 * np.power(np.clip(result, 0, 255) / 255, rng.uniform(0.7, 1.4))

    if rng.random() < 0.3:
        height, width = result.shape[:2]
        ys, xs = np.mgrid[0:height, 0:width]
        distance = np.hypot((xs - width / 2) / width, (ys - height / 2) / height)
        result *= (1 - rng.uniform(0.1, 0.5) * distance**2 * 2)[..., None]

    result = np.clip(result, 0, 255).astype(np.uint8)

    if rng.random() < 0.5:
        factor = rng.uniform(0.5, 0.95)
        small = cv2.resize(
            result, None, fx=factor, fy=factor, interpolation=cv2.INTER_AREA
        )
        result = cv2.resize(
            small, (image.shape[1], image.shape[0]), interpolation=cv2.INTER_LINEAR
        )

    if rng.random() < 0.6:
        result = cv2.GaussianBlur(result, (0, 0), rng.uniform(0.3, 1.6))

    if rng.random() < 0.15:
        size = int(rng.integers(5, 12))
        kernel = np.zeros((size, size), np.float32)
        kernel[size // 2, :] = 1 / size
        matrix = cv2.getRotationMatrix2D(
            (size / 2 - 0.5, size / 2 - 0.5), rng.uniform(0, 180), 1
        )
        kernel = cv2.warpAffine(kernel, matrix, (size, size))
        result = cv2.filter2D(result, -1, kernel / max(kernel.sum(), 1e-6))

    noise = rng.normal(0, rng.uniform(0, 8), result.shape)
    result = np.clip(result.astype(np.float32) + noise, 0, 255).astype(np.uint8)
    return result


def generate(job):
    seed, split, output = job
    rng = np.random.default_rng(seed)
    image, _, labels = render_scene(rng, DECKS)
    image = photometric(image, rng)

    height, width = image.shape[:2]
    name = f"{seed:07d}"
    image_path = os.path.join(output, "images", split, f"{name}.jpg")
    cv2.imwrite(
        image_path, image, [cv2.IMWRITE_JPEG_QUALITY, int(rng.integers(55, 96))]
    )

    lines = []
    for class_id, left, top, right, bottom in labels:
        cx, cy = (left + right) / 2 / width, (top + bottom) / 2 / height
        lines.append(
            f"{class_id} {cx:.6f} {cy:.6f} {(right - left) / width:.6f} {(bottom - top) / height:.6f}"
        )

    with open(os.path.join(output, "labels", split, f"{name}.txt"), "w") as handle:
        handle.write("\n".join(lines))

    return len(labels)


def write_debug(output, count, seed):
    os.makedirs(os.path.join(output, "debug"), exist_ok=True)
    for index in range(count):
        rng = np.random.default_rng(seed + index)
        image, _, labels = render_scene(rng, DECKS)
        image = photometric(image, rng)
        for class_id, left, top, right, bottom in labels:
            cv2.rectangle(image, (left, top), (right, bottom), (0, 255, 0), 2)
            cv2.putText(
                image,
                CARDS[class_id],
                (left, max(15, top - 4)),
                cv2.FONT_HERSHEY_SIMPLEX,
                0.7,
                (0, 0, 255),
                2,
            )
        cv2.imwrite(os.path.join(output, "debug", f"{index}.jpg"), image)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default=os.path.join(ROOT, "data"))
    parser.add_argument("--train", type=int, default=5000)
    parser.add_argument("--val", type=int, default=500)
    parser.add_argument("--seed", type=int, default=0)
    parser.add_argument("--workers", type=int, default=os.cpu_count())
    parser.add_argument("--debug", type=int, default=0)
    args = parser.parse_args()

    if args.debug:
        write_debug(args.out, args.debug, args.seed + 1_000_000)
        return

    jobs = []
    for split, count, base in (
        ("train", args.train, args.seed),
        ("val", args.val, args.seed + 900_000),
    ):
        os.makedirs(os.path.join(args.out, "images", split), exist_ok=True)
        os.makedirs(os.path.join(args.out, "labels", split), exist_ok=True)
        jobs += [(base + index, split, args.out) for index in range(count)]

    with open(os.path.join(args.out, "data.yaml"), "w") as handle:
        handle.write(
            f"path: {os.path.abspath(args.out)}\ntrain: images/train\nval: images/val\nnames:\n"
        )
        handle.writelines(f"  {index}: {card}\n" for index, card in enumerate(CARDS))

    total = 0
    with Pool(args.workers) as pool:
        for done, count in enumerate(
            pool.imap_unordered(generate, jobs, chunksize=8), start=1
        ):
            total += count
            if done % 250 == 0:
                print(f"{done}/{len(jobs)} images, {total} boxes", flush=True)
    print(f"done: {len(jobs)} images, {total} boxes")


if __name__ == "__main__":
    sys.exit(main())
