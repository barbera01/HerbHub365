#!/usr/bin/env python3

import sys
import time
import gpiod
from gpiod.line import Direction, Value

CHIP = "/dev/gpiochip0"

RELAYS = {
    "BASIL": 26,
    "CHILLI": 20,
    "OREGANO": 21,
}

ON_VALUE = Value.INACTIVE
OFF_VALUE = Value.ACTIVE


def usage():
    print("Usage:")
    print("  sudo python3 relay.py CH1 on")
    print("  sudo python3 relay.py CH2 off")
    print("  sudo python3 relay.py CH3 pulse 3")
    print("  sudo python3 relay.py all on")
    print("  sudo python3 relay.py all off")
    print("  sudo python3 relay.py cycle 30 3")
    print("  sudo python3 relay.py pulsecombo CH1 CH2 3")
    print("  sudo python3 relay.py pulsecombo CH1 CH2 CH3 2")
    sys.exit(1)


def build_config():
    return {
        pin: gpiod.LineSettings(
            direction=Direction.OUTPUT,
            output_value=OFF_VALUE,
        )
        for pin in RELAYS.values()
    }


def set_one(req, channel, state):
    req.set_value(RELAYS[channel], state)


def set_many(req, channels, state):
    for channel in channels:
        req.set_value(RELAYS[channel], state)


def set_all(req, state):
    for pin in RELAYS.values():
        req.set_value(pin, state)


def main():
    if len(sys.argv) < 2:
        usage()

    with gpiod.request_lines(
        CHIP,
        consumer="relay-control",
        config=build_config(),
    ) as req:

        set_all(req, OFF_VALUE)

        cmd = sys.argv[1].upper()

        if cmd == "CYCLE":
            if len(sys.argv) < 4:
                usage()

            total_seconds = float(sys.argv[2])
            on_seconds = float(sys.argv[3])

            start = time.time()

            while time.time() - start < total_seconds:
                for channel in RELAYS:
                    if time.time() - start >= total_seconds:
                        break

                    set_one(req, channel, ON_VALUE)
                    print(f"{channel} ON")
                    time.sleep(on_seconds)

                    set_one(req, channel, OFF_VALUE)
                    print(f"{channel} OFF")

            set_all(req, OFF_VALUE)
            print("Cycle finished")
            return

        if cmd == "PULSECOMBO":
            if len(sys.argv) < 4:
                usage()

            channels = [arg.upper() for arg in sys.argv[2:-1]]
            seconds = float(sys.argv[-1])

            for channel in channels:
                if channel not in RELAYS:
                    print(f"Invalid channel: {channel}")
                    usage()

            print(f"{', '.join(channels)} ON for {seconds} seconds")
            set_many(req, channels, ON_VALUE)
            time.sleep(seconds)
            set_many(req, channels, OFF_VALUE)
            print(f"{', '.join(channels)} OFF")
            return

        if len(sys.argv) < 3:
            usage()

        target = sys.argv[1].upper()
        action = sys.argv[2].lower()

        if target in RELAYS:
            if action == "on":
                set_one(req, target, ON_VALUE)
                print(f"{target} ON")
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    set_one(req, target, OFF_VALUE)
                    print(f"{target} OFF")

            elif action == "off":
                set_one(req, target, OFF_VALUE)
                print(f"{target} OFF")

            elif action == "pulse":
                if len(sys.argv) < 4:
                    usage()

                seconds = float(sys.argv[3])
                set_one(req, target, ON_VALUE)
                print(f"{target} ON for {seconds} seconds")
                time.sleep(seconds)
                set_one(req, target, OFF_VALUE)
                print(f"{target} OFF")
            else:
                usage()

        elif target == "ALL":
            if action == "on":
                set_all(req, ON_VALUE)
                print("ALL ON")
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    set_all(req, OFF_VALUE)
                    print("ALL OFF")

            elif action == "off":
                set_all(req, OFF_VALUE)
                print("ALL OFF")
            else:
                usage()

        else:
            usage()


if __name__ == "__main__":
    main()
