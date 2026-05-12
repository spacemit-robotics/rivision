# Copyright (c) 2023 SpacemiT. All rights reserved.
import torch
import torchvision.datasets as datasets
import torchvision.transforms as transforms
import os
import shutil
import numpy as np
from PIL import Image
import argparse
import cv2
from torch.utils.data.dataset import Subset

parser = argparse.ArgumentParser()
parser.add_argument(
    "--dataset_dir", required=True, help="Path to the Imagenet Test directory."
)
parser.add_argument("--output_dir", required=True, help="Path to the output directory.")
parser.add_argument(
    "--subset",
    required=False,
    type=int,
    default=None,
    help="subset num to the Imagenet Test Dataset.",
)
parser.add_argument(
    "--size",
    required=False,
    type=int,
    default=224,
    help="img size, default to 224.",
)
parser.add_argument(
    "--color_format",
    required=False,
    type=str,
    default="rgb",
    help="color format rgb or bgr, default to rgb",
)
parser.add_argument(
    "--tf_preprocess",
    action="store_true",
    help="enable tf_preprocess.",
)

args = parser.parse_args()

TEST_DIR = args.dataset_dir
DEST_RAW_IMG_DIR = args.output_dir
DEST_RAW_IMG_LIST_PATH = os.path.join(DEST_RAW_IMG_DIR, "img_list.txt")
subset_num = args.subset
resize = args.size
color_format = args.color_format
tf_preprocess = args.tf_preprocess

if os.path.exists(DEST_RAW_IMG_DIR):
    shutil.rmtree(DEST_RAW_IMG_DIR)
os.makedirs(DEST_RAW_IMG_DIR)

print("resize {}\n".format(resize))
print("color_format {}\n".format(color_format))
print("tf_preprocess {}\n".format(tf_preprocess))


class PTImagenetPreprocess:
    def __init__(self, out_height, out_width):
        self.out_height = out_height
        self.out_width = out_width

    def __call__(self, img):
        import torchvision.transforms.functional as F

        img = Image.fromarray(img)
        img = F.resize(img, int(self.out_height / 0.875), Image.BILINEAR)
        img = F.center_crop(img, self.out_height)
        img = np.array(img)
        img = np.transpose(img, (2, 0, 1))
        return img


class ImagenetPreprocess:
    def resize_with_aspectratio(
        self,
        img: np.ndarray,
        out_height: int,
        out_width: int,
        scale: float = 87.5,
        inter_pol=cv2.INTER_LINEAR,
    ):
        height, width, _ = img.shape
        new_height = int(100.0 * out_height / scale)
        new_width = int(100.0 * out_width / scale)
        if height > width:
            w = new_width
            h = int(new_height * height / width)
        else:
            h = new_height
            w = int(new_width * width / height)
        img = cv2.resize(img, (w, h), interpolation=inter_pol)
        return img

    def center_crop(self, img: np.ndarray, out_height: int, out_width: int):
        height, width, _ = img.shape
        left = int((width - out_width) / 2)
        right = int((width + out_width) / 2)
        top = int((height - out_height) / 2)
        bottom = int((height + out_height) / 2)
        img = img[top:bottom, left:right]
        return img

    def __init__(self, out_height: int, out_width: int):
        self.out_height = out_height
        self.out_width = out_width

    def __call__(self, img):
        cv2_interpol = cv2.INTER_AREA
        img = self.resize_with_aspectratio(
            img, self.out_height, self.out_width, inter_pol=cv2_interpol
        )
        img = self.center_crop(img, self.out_height, self.out_width)
        img = np.transpose(img, (2, 0, 1))
        return img


def custom_loader(x):
    img = cv2.imread(x)
    if color_format == "rgb":
        img = cv2.cvtColor(img, cv2.COLOR_BGR2RGB)
    return img


dataset = datasets.ImageFolder(
    TEST_DIR,
    transforms.Compose(
        [
            PTImagenetPreprocess(resize, resize)
            if not tf_preprocess
            else ImagenetPreprocess(resize, resize),
        ]
    ),
    loader=custom_loader,
)

if subset_num is not None:
    total_num = len(dataset)
    dataset = Subset(dataset, indices=[_ for _ in range(0, total_num, total_num // subset_num)])

dataloader = torch.utils.data.DataLoader(
    dataset=dataset,
    batch_size=1,
    shuffle=False,
    num_workers=0,
    pin_memory=False,
    drop_last=True,
)

wt_lines = []
for idx, data_item in enumerate(dataloader):
    img_data, label_idx = data_item
    file_path = os.path.realpath(
        os.path.join(
            DEST_RAW_IMG_DIR,
            "imagenet_raw_data_{}_cls_{}.bin".format(idx, int(label_idx)),
        )
    )
    npy_file_path = os.path.realpath(
        os.path.join(
            DEST_RAW_IMG_DIR,
            "imagenet_raw_data_{}_cls_{}.npy".format(idx, int(label_idx)),
        )
    )
    img_data_np = img_data.numpy()
    img_data_np.tofile(file_path)
    np.save(npy_file_path, img_data_np)
    wt_lines.append("{},{}\n".format(os.path.basename(file_path), int(label_idx)))


with open(DEST_RAW_IMG_LIST_PATH, "w") as fp:
    fp.writelines(wt_lines)
