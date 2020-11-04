# coding=utf-8
# *** WARNING: this file was generated by test. ***
# *** Do not edit by hand unless you're certain you know what you are doing! ***

from enum import Enum

__all__ = [
    'ContainerBrightness',
    'ContainerColor',
    'ContainerSize',
]


class ContainerBrightness(float, Enum):
    ZERO_POINT_ONE = 0.1
    ONE = 1.0


class ContainerColor(str, Enum):
    """
    plant container colors
    """
    RED = "red"
    BLUE = "blue"
    YELLOW = "yellow"


class ContainerSize(int, Enum):
    """
    plant container sizes
    """
    FOUR_INCH = 4
    SIX_INCH = 6
    EIGHT_INCH = 8
