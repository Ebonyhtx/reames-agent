"""Minimal screenshot-free Windows UI Automation client for Desktop smoke tests.

The driver uses the platform UIA COM interfaces directly through ``ctypes``.
It prefers InvokePattern/ValuePattern and falls back to the element's UIA
bounding rectangle only when a WebView button does not expose InvokePattern.
"""

from __future__ import annotations

import ctypes
import sys
import time
import uuid
from ctypes import wintypes
from dataclasses import dataclass
from typing import Iterable


TREE_SCOPE_DESCENDANTS = 0x4
UIA_INVOKE_PATTERN_ID = 10000
UIA_VALUE_PATTERN_ID = 10002

NEW_SESSION_NAMES = ("New session", "新建会话")
SEND_NAMES = ("Send (Enter)", "发送（Enter）")
STOP_NAMES = ("Stop (Esc)", "Stop", "停止（Esc）", "停止")


class GUID(ctypes.Structure):
    _fields_ = [
        ("Data1", ctypes.c_ulong),
        ("Data2", ctypes.c_ushort),
        ("Data3", ctypes.c_ushort),
        ("Data4", ctypes.c_ubyte * 8),
    ]

    @classmethod
    def parse(cls, value: str) -> "GUID":
        return cls.from_buffer_copy(uuid.UUID(value).bytes_le)


CLSID_CUI_AUTOMATION = GUID.parse("ff48dba4-60ef-4201-aa87-54103eef594e")
IID_IUI_AUTOMATION = GUID.parse("30cbe57d-d9d0-452a-ab13-7ac5ac4825ee")
IID_INVOKE_PATTERN = GUID.parse("fb377fbe-8ea6-46d5-9c73-6499642d3059")
IID_VALUE_PATTERN = GUID.parse("a94cd8b1-0844-4cd6-9d2d-640537ab39e9")


@dataclass(frozen=True)
class ElementInfo:
    index: int
    name: str
    automation_id: str
    control_type: int
    enabled: bool


class _MouseInput(ctypes.Structure):
    _fields_ = [
        ("dx", wintypes.LONG),
        ("dy", wintypes.LONG),
        ("mouseData", wintypes.DWORD),
        ("dwFlags", wintypes.DWORD),
        ("time", wintypes.DWORD),
        ("dwExtraInfo", ctypes.c_size_t),
    ]


class _KeyboardInput(ctypes.Structure):
    _fields_ = [
        ("wVk", wintypes.WORD),
        ("wScan", wintypes.WORD),
        ("dwFlags", wintypes.DWORD),
        ("time", wintypes.DWORD),
        ("dwExtraInfo", ctypes.c_size_t),
    ]


class _HardwareInput(ctypes.Structure):
    _fields_ = [
        ("uMsg", wintypes.DWORD),
        ("wParamL", wintypes.WORD),
        ("wParamH", wintypes.WORD),
    ]


class _InputUnion(ctypes.Union):
    _fields_ = [
        ("mi", _MouseInput),
        ("ki", _KeyboardInput),
        ("hi", _HardwareInput),
    ]


class _Input(ctypes.Structure):
    _fields_ = [("type", wintypes.DWORD), ("data", _InputUnion)]


def _require_windows() -> None:
    if sys.platform != "win32":
        raise RuntimeError("Windows UI Automation requires win32")


def _check_hresult(hr: int, operation: str) -> None:
    if hr < 0:
        raise OSError(hr, operation)


def _method(obj: ctypes.c_void_p, index: int, *argtypes: object):
    table = ctypes.cast(obj, ctypes.POINTER(ctypes.POINTER(ctypes.c_void_p))).contents
    return ctypes.WINFUNCTYPE(ctypes.c_long, ctypes.c_void_p, *argtypes)(table[index])


def _release(obj: ctypes.c_void_p | None) -> None:
    if obj and obj.value:
        _method(obj, 2)(obj)


def _visible_windows_for_pid(pid: int) -> list[int]:
    _require_windows()
    user32 = ctypes.windll.user32
    callback_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)
    found: list[int] = []

    def callback(hwnd: int, _param: int) -> bool:
        owner = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(owner))
        if owner.value == pid and user32.IsWindowVisible(hwnd):
            found.append(int(hwnd))
        return True

    callback_ref = callback_type(callback)
    if not user32.EnumWindows(callback_ref, 0):
        error = ctypes.get_last_error()
        if error:
            raise OSError(error, "EnumWindows failed")
    return found


def wait_for_window(pid: int, timeout_seconds: float = 20.0) -> int:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        windows = _visible_windows_for_pid(pid)
        if windows:
            return windows[0]
        time.sleep(0.1)
    raise TimeoutError(f"no visible window appeared for pid {pid}")


class WindowsUIAutomation:
    """Owns one UIA COM session rooted at a native window handle."""

    def __init__(self, hwnd: int):
        _require_windows()
        self.hwnd = hwnd
        self.actions: list[dict[str, object]] = []
        self._automation = ctypes.c_void_p()
        self._root = ctypes.c_void_p()
        self._condition = ctypes.c_void_p()
        self._elements: list[dict[str, object]] = []
        self._closed = False
        self._oleaut32 = ctypes.windll.oleaut32
        self._oleaut32.SysAllocString.argtypes = [ctypes.c_wchar_p]
        self._oleaut32.SysAllocString.restype = ctypes.c_void_p
        self._oleaut32.SysFreeString.argtypes = [ctypes.c_void_p]
        self._oleaut32.SysFreeString.restype = None

        try:
            ctypes.windll.user32.SetProcessDpiAwarenessContext(ctypes.c_void_p(-4))
        except (AttributeError, OSError):
            ctypes.windll.user32.SetProcessDPIAware()

        _check_hresult(ctypes.windll.ole32.CoInitializeEx(None, 2), "CoInitializeEx")
        try:
            _check_hresult(
                ctypes.windll.ole32.CoCreateInstance(
                    ctypes.byref(CLSID_CUI_AUTOMATION),
                    None,
                    1,
                    ctypes.byref(IID_IUI_AUTOMATION),
                    ctypes.byref(self._automation),
                ),
                "CoCreateInstance(CUIAutomation)",
            )
            _check_hresult(
                _method(
                    self._automation,
                    6,
                    wintypes.HWND,
                    ctypes.POINTER(ctypes.c_void_p),
                )(self._automation, hwnd, ctypes.byref(self._root)),
                "ElementFromHandle",
            )
            _check_hresult(
                _method(self._automation, 21, ctypes.POINTER(ctypes.c_void_p))(
                    self._automation, ctypes.byref(self._condition)
                ),
                "CreateTrueCondition",
            )
        except Exception:
            self.close()
            raise

    def __enter__(self) -> "WindowsUIAutomation":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        self._release_elements()
        _release(self._condition)
        _release(self._root)
        _release(self._automation)
        ctypes.windll.ole32.CoUninitialize()

    def _release_elements(self) -> None:
        for item in self._elements:
            _release(item["element"])  # type: ignore[arg-type]
        self._elements.clear()

    def _current_bstr(self, element: ctypes.c_void_p, index: int) -> str:
        value = ctypes.c_void_p()
        _check_hresult(
            _method(element, index, ctypes.POINTER(ctypes.c_void_p))(
                element, ctypes.byref(value)
            ),
            "read UIA BSTR",
        )
        if not value.value:
            return ""
        try:
            return ctypes.wstring_at(value.value)
        finally:
            self._oleaut32.SysFreeString(value)

    @staticmethod
    def _current_int(element: ctypes.c_void_p, index: int) -> int:
        value = ctypes.c_int()
        _check_hresult(
            _method(element, index, ctypes.POINTER(ctypes.c_int))(
                element, ctypes.byref(value)
            ),
            "read UIA integer property",
        )
        return value.value

    def refresh(self) -> list[ElementInfo]:
        self._release_elements()
        array = ctypes.c_void_p()
        _check_hresult(
            _method(
                self._root,
                6,
                ctypes.c_int,
                ctypes.c_void_p,
                ctypes.POINTER(ctypes.c_void_p),
            )(
                self._root,
                TREE_SCOPE_DESCENDANTS,
                self._condition,
                ctypes.byref(array),
            ),
            "FindAll",
        )
        try:
            length = ctypes.c_int()
            _check_hresult(
                _method(array, 3, ctypes.POINTER(ctypes.c_int))(
                    array, ctypes.byref(length)
                ),
                "ElementArray.Length",
            )
            for index in range(length.value):
                element = ctypes.c_void_p()
                _check_hresult(
                    _method(array, 4, ctypes.c_int, ctypes.POINTER(ctypes.c_void_p))(
                        array, index, ctypes.byref(element)
                    ),
                    "ElementArray.GetElement",
                )
                try:
                    info = ElementInfo(
                        index=index,
                        name=self._current_bstr(element, 23),
                        automation_id=self._current_bstr(element, 29),
                        control_type=self._current_int(element, 21),
                        enabled=bool(self._current_int(element, 28)),
                    )
                except OSError:
                    _release(element)
                    continue
                self._elements.append({"info": info, "element": element})
        finally:
            _release(array)
        return [item["info"] for item in self._elements]  # type: ignore[misc]

    @staticmethod
    def _names(value: str | Iterable[str]) -> tuple[str, ...]:
        if isinstance(value, str):
            return () if value == "" else (value,)
        return tuple(value)

    def _find(
        self,
        *,
        name: str | Iterable[str] = "",
        automation_id: str = "",
        occurrence: int = 0,
        timeout_seconds: float = 10.0,
    ) -> dict[str, object]:
        names = self._names(name)
        deadline = time.monotonic() + timeout_seconds
        while True:
            self.refresh()
            matches = [
                item
                for item in self._elements
                if (not names or item["info"].name in names)  # type: ignore[union-attr]
                and (
                    not automation_id
                    or item["info"].automation_id == automation_id  # type: ignore[union-attr]
                )
            ]
            if matches:
                if -len(matches) <= occurrence < len(matches):
                    return matches[occurrence]
                raise RuntimeError(
                    f"UIA occurrence {occurrence} unavailable for {names or automation_id!r}; "
                    f"found {len(matches)}"
                )
            if time.monotonic() >= deadline:
                raise TimeoutError(
                    f"UIA element not found: names={names!r} automation_id={automation_id!r}"
                )
            time.sleep(0.1)

    def has(
        self,
        *,
        name: str | Iterable[str] = "",
        automation_id: str = "",
    ) -> bool:
        names = self._names(name)
        return any(
            (not names or item.name in names)
            and (not automation_id or item.automation_id == automation_id)
            for item in self.refresh()
        )

    def wait_absent(
        self,
        *,
        name: str | Iterable[str] = "",
        automation_id: str = "",
        timeout_seconds: float = 10.0,
    ) -> None:
        deadline = time.monotonic() + timeout_seconds
        while time.monotonic() < deadline:
            if not self.has(name=name, automation_id=automation_id):
                return
            time.sleep(0.1)
        raise TimeoutError(f"UIA element remained visible: {name or automation_id!r}")

    def wait_enabled(
        self,
        *,
        name: str | Iterable[str] = "",
        automation_id: str = "",
        timeout_seconds: float = 10.0,
    ) -> ElementInfo:
        """Wait until a matching control is both present and enabled."""

        names = self._names(name)
        deadline = time.monotonic() + timeout_seconds
        while time.monotonic() < deadline:
            for item in self.refresh():
                if (
                    (not names or item.name in names)
                    and (not automation_id or item.automation_id == automation_id)
                    and item.enabled
                ):
                    return item
            time.sleep(0.1)
        raise TimeoutError(
            f"UIA element did not become enabled: {names or automation_id!r}"
        )

    def invoke(
        self,
        *,
        name: str | Iterable[str] = "",
        automation_id: str = "",
        occurrence: int = 0,
        timeout_seconds: float = 10.0,
    ) -> str:
        item = self._find(
            name=name,
            automation_id=automation_id,
            occurrence=occurrence,
            timeout_seconds=timeout_seconds,
        )
        info: ElementInfo = item["info"]  # type: ignore[assignment]
        element: ctypes.c_void_p = item["element"]  # type: ignore[assignment]
        pattern = ctypes.c_void_p()
        hr = _method(
            element,
            14,
            ctypes.c_int,
            ctypes.POINTER(GUID),
            ctypes.POINTER(ctypes.c_void_p),
        )(
            element,
            UIA_INVOKE_PATTERN_ID,
            ctypes.byref(IID_INVOKE_PATTERN),
            ctypes.byref(pattern),
        )
        if hr >= 0 and pattern.value:
            try:
                invoke_hr = _method(pattern, 3)(pattern)
            finally:
                _release(pattern)
            if invoke_hr >= 0:
                action = "invoke-pattern"
            else:
                action = ""
        else:
            action = ""
        if not action:
            rect = wintypes.RECT()
            _check_hresult(
                _method(element, 43, ctypes.POINTER(wintypes.RECT))(
                    element, ctypes.byref(rect)
                ),
                f"BoundingRectangle {info.name!r}",
            )
            if rect.right <= rect.left or rect.bottom <= rect.top:
                raise RuntimeError(f"UIA element has no clickable bounds: {info.name!r}")
            if not ctypes.windll.user32.SetForegroundWindow(self.hwnd):
                raise OSError("SetForegroundWindow failed")
            x = (rect.left + rect.right) // 2
            y = (rect.top + rect.bottom) // 2
            if not ctypes.windll.user32.SetCursorPos(x, y):
                raise OSError("SetCursorPos failed")
            ctypes.windll.user32.mouse_event(0x0002, 0, 0, 0, 0)
            ctypes.windll.user32.mouse_event(0x0004, 0, 0, 0, 0)
            action = "uia-bounds-click"
        self.actions.append(
            {
                "action": action,
                "name": info.name,
                "automation_id": info.automation_id,
                "occurrence": occurrence,
            }
        )
        return action

    def set_value(
        self,
        value: str,
        *,
        automation_id: str,
        timeout_seconds: float = 10.0,
    ) -> None:
        item = self._find(
            automation_id=automation_id,
            timeout_seconds=timeout_seconds,
        )
        info: ElementInfo = item["info"]  # type: ignore[assignment]
        element: ctypes.c_void_p = item["element"]  # type: ignore[assignment]
        pattern = ctypes.c_void_p()
        _check_hresult(
            _method(
                element,
                14,
                ctypes.c_int,
                ctypes.POINTER(GUID),
                ctypes.POINTER(ctypes.c_void_p),
            )(
                element,
                UIA_VALUE_PATTERN_ID,
                ctypes.byref(IID_VALUE_PATTERN),
                ctypes.byref(pattern),
            ),
            f"GetCurrentPatternAs(Value) {info.name!r}",
        )
        if not pattern.value:
            raise RuntimeError(f"UIA ValuePattern unavailable: {info.name!r}")
        bstr = self._oleaut32.SysAllocString(value)
        if not bstr:
            _release(pattern)
            raise MemoryError("SysAllocString")
        try:
            _check_hresult(
                _method(pattern, 3, ctypes.c_void_p)(pattern, bstr),
                f"SetValue {info.name!r}",
            )
        finally:
            self._oleaut32.SysFreeString(bstr)
            _release(pattern)
        self.actions.append(
            {
                "action": "value-pattern",
                "name": info.name,
                "automation_id": info.automation_id,
                "value_length": len(value),
            }
        )

    @staticmethod
    def _keyboard_input(vk: int, scan: int, flags: int) -> _Input:
        return _Input(
            type=1,
            data=_InputUnion(
                ki=_KeyboardInput(
                    wVk=vk,
                    wScan=scan,
                    dwFlags=flags,
                    time=0,
                    dwExtraInfo=0,
                )
            ),
        )

    @staticmethod
    def _send_inputs(inputs: list[_Input]) -> None:
        if not inputs:
            return
        array_type = _Input * len(inputs)
        array = array_type(*inputs)
        sent = ctypes.windll.user32.SendInput(
            len(inputs), ctypes.byref(array), ctypes.sizeof(_Input)
        )
        if sent != len(inputs):
            raise OSError(ctypes.get_last_error(), f"SendInput sent {sent}/{len(inputs)}")

    def _focus(self, item: dict[str, object]) -> ElementInfo:
        info: ElementInfo = item["info"]  # type: ignore[assignment]
        element: ctypes.c_void_p = item["element"]  # type: ignore[assignment]
        if not ctypes.windll.user32.SetForegroundWindow(self.hwnd):
            raise OSError("SetForegroundWindow failed")
        time.sleep(0.1)
        _check_hresult(_method(element, 3)(element), f"SetFocus {info.name!r}")
        deadline = time.monotonic() + 2.0
        while time.monotonic() < deadline:
            try:
                if self._current_int(element, 26):
                    break
            except OSError:
                pass
            time.sleep(0.05)
        else:
            raise RuntimeError(f"UIA element did not receive keyboard focus: {info.name!r}")
        time.sleep(0.05)
        return info

    def type_text(
        self,
        value: str,
        *,
        automation_id: str,
        timeout_seconds: float = 10.0,
    ) -> None:
        item = self._find(
            automation_id=automation_id,
            timeout_seconds=timeout_seconds,
        )
        info = self._focus(item)
        self._send_inputs(
            [
                self._keyboard_input(0x11, 0, 0),
                self._keyboard_input(0x41, 0, 0),
                self._keyboard_input(0x41, 0, 0x0002),
                self._keyboard_input(0x11, 0, 0x0002),
                self._keyboard_input(0x08, 0, 0),
                self._keyboard_input(0x08, 0, 0x0002),
            ]
        )
        units = value.encode("utf-16-le")
        inputs: list[_Input] = []
        for offset in range(0, len(units), 2):
            scan = int.from_bytes(units[offset : offset + 2], "little")
            inputs.append(self._keyboard_input(0, scan, 0x0004))
            inputs.append(self._keyboard_input(0, scan, 0x0004 | 0x0002))
        self._send_inputs(inputs)
        self.actions.append(
            {
                "action": "uia-focus-sendinput",
                "name": info.name,
                "automation_id": info.automation_id,
                "value_length": len(value),
            }
        )

    def press_enter(
        self,
        *,
        automation_id: str,
        timeout_seconds: float = 10.0,
    ) -> None:
        item = self._find(
            automation_id=automation_id,
            timeout_seconds=timeout_seconds,
        )
        info = self._focus(item)
        self._send_inputs(
            [
                self._keyboard_input(0x0D, 0, 0),
                self._keyboard_input(0x0D, 0, 0x0002),
            ]
        )
        self.actions.append(
            {
                "action": "uia-focus-enter",
                "name": info.name,
                "automation_id": info.automation_id,
            }
        )
