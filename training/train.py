import argparse
import os

from ultralytics import YOLO

ROOT = os.path.dirname(os.path.abspath(__file__))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", default="yolov8s.pt")
    parser.add_argument("--epochs", type=int, default=40)
    parser.add_argument("--imgsz", type=int, default=960)
    parser.add_argument("--batch", type=int, default=16)
    parser.add_argument("--fraction", type=float, default=1.0)
    parser.add_argument("--name", default="corners")
    parser.add_argument("--device", default="mps")
    args = parser.parse_args()

    model = YOLO(args.model)
    model.train(
        data=os.path.join(ROOT, "data", "data.yaml"),
        epochs=args.epochs,
        imgsz=args.imgsz,
        batch=args.batch,
        fraction=args.fraction,
        device=args.device,
        workers=8,
        project=os.path.join(ROOT, "runs"),
        name=args.name,
        exist_ok=True,
        fliplr=0.0,
        flipud=0.0,
        degrees=0.0,
        hsv_h=0.0,
        hsv_s=0.5,
        hsv_v=0.4,
        scale=0.4,
        mosaic=0.3,
        erasing=0.1,
        patience=15,
        plots=False,
    )


if __name__ == "__main__":
    main()
