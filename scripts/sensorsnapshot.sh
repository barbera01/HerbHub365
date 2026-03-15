#!/usr/bin/env bash
I2C_BUS=1
ADS_ADDR=0x48
BME280_ADDR=0x76
BH1750_ADDR=0x23
CFG_LOW=0x83
TRIGGER_PIN=18
ECHO_PIN=27
PULSES_PER_LITRE=5880
# Water tank calibration (IKEA KARAFF carafe - 1L, 28cm height)
EMPTY_DISTANCE=26.0  # Distance when carafe is empty (adjust after measuring)
FULL_DISTANCE=4.0    # Distance when carafe is full at 1L (adjust after measuring)
declare -A TEMP_MAP=(
["28-0000006d0b68"]="basil"
["28-000000671c2b"]="oregano"
["28-00000071bd22"]="chilli"
["28-0000007131c5"]="water"
)
declare -A MOISTURE_MAP=(
["A0"]="oregano"
["A1"]="chilli"
["A2"]="basil"
)
declare -A CH_CFG_HIGH=(
[A0]=0xC3
[A1]=0xD3
[A2]=0xE3
[A3]=0xF3
)
declare -A DRY_CAL=(
["oregano"]=2.170
["chilli"]=2.137
["basil"]=2.131
)
declare -A WET_CAL=(
["oregano"]=1.014
["chilli"]=0.828
["basil"]=0.928
)
read_ads() {
    chan=$1
    i2cset -y $I2C_BUS $ADS_ADDR 0x01 ${CH_CFG_HIGH[$chan]} $CFG_LOW i >/dev/null
    sleep 0.01
    raw=$(i2cget -y $I2C_BUS $ADS_ADDR 0x00 w)
    raw=${raw#0x}
    swapped="${raw:2:2}${raw:0:2}"
    val=$((16#$swapped))
    if [ $val -gt 32767 ]; then
        val=$((val-65536))
    fi
    echo "scale=4; $val * 4.096 / 32768" | bc -l
}
read_temp() {
    file=$1/w1_slave
    if ! grep -q YES "$file"; then
        echo "nan"
        return
    fi
    t=$(grep -o "t=-*[0-9]*" "$file" | cut -d= -f2)
    echo "scale=3; $t/1000" | bc -l
}
read_bh1750() {
    python3 << 'EOF'
import smbus
import time
bus = smbus.SMBus(1)
addr = 0x23
try:
    # Continuous H-resolution mode
    bus.write_byte(addr, 0x10)
    time.sleep(0.2)
    
    # Read data
    data = bus.read_i2c_block_data(addr, 0x00, 2)
    lux = (data[0] * 256 + data[1]) / 1.2
    print(f"{lux:.1f}")
except:
    print("error")
EOF
}
read_bme280() {
    python3 << 'EOF'
import board
from adafruit_bme280 import basic as adafruit_bme280
i2c = board.I2C()
bme280 = adafruit_bme280.Adafruit_BME280_I2C(i2c, address=0x76)
print(f'Temperature: {bme280.temperature:.2f}C')
print(f'Humidity: {bme280.relative_humidity:.1f}%')
print(f'Pressure: {bme280.pressure:.0f}hPa')
EOF
}
read_hcsr04t() {
    python3 << 'EOF'
import RPi.GPIO as GPIO
import time
import statistics
TRIGGER = 18
ECHO = 27
GPIO.setmode(GPIO.BCM)
GPIO.setup(TRIGGER, GPIO.OUT)
GPIO.setup(ECHO, GPIO.IN)
readings = []
# Take 5 readings
for i in range(5):
    GPIO.output(TRIGGER, False)
    time.sleep(0.05)
    GPIO.output(TRIGGER, True)
    time.sleep(0.00001)
    GPIO.output(TRIGGER, False)
    
    timeout = time.time() + 1.0
    pulse_start = time.time()
    
    while GPIO.input(ECHO) == 0:
        pulse_start = time.time()
        if pulse_start > timeout:
            break
    
    pulse_end = pulse_start
    while GPIO.input(ECHO) == 1:
        pulse_end = time.time()
        if pulse_end > timeout:
            break
    
    if pulse_end > pulse_start:
        pulse_duration = pulse_end - pulse_start
        distance = pulse_duration * 17150
        if 2 < distance < 400:
            readings.append(distance)
    
    time.sleep(0.05)
GPIO.cleanup()
if len(readings) >= 3:
    median_distance = statistics.median(readings)
    print(f"{median_distance:.2f}")
else:
    print("error")
EOF
}
calc_water_percent() {
    distance=$1
    
    # Invert: smaller distance = more water
    percent=$(echo "scale=1; (($EMPTY_DISTANCE - $distance) / ($EMPTY_DISTANCE - $FULL_DISTANCE)) * 100" | bc -l)
    
    # Clamp 0-100
    if (( $(echo "$percent < 0" | bc -l) )); then
        percent=0
    fi
    if (( $(echo "$percent > 100" | bc -l) )); then
        percent=100
    fi
    
    echo "$percent"
}
calc_water_volume() {
    percent=$1
    
    # 1L carafe, calculate volume based on percentage
    volume=$(echo "scale=0; $percent * 10" | bc -l)
    
    echo "$volume"
}
calc_moisture() {
    plant=$1
    voltage=$2
    dry=${DRY_CAL[$plant]}
    wet=${WET_CAL[$plant]}
    m=$(echo "scale=4; ($dry - $voltage) / ($dry - $wet)" | bc -l)
    if (( $(echo "$m < 0" | bc -l) )); then
        m=0
    fi
    if (( $(echo "$m > 1" | bc -l) )); then
        m=1
    fi
    echo "scale=1; $m * 100" | bc -l
}

write_json_snapshot() {
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    # Get all sensor data
    bme_temp=""
    bme_humidity=""
    bme_pressure=""
    bme_output=$(read_bme280 2>/dev/null)
    if [ -n "$bme_output" ]; then
        bme_temp=$(echo "$bme_output" | grep "Temperature" | awk '{print $2}' | tr -d 'C')
        bme_humidity=$(echo "$bme_output" | grep "Humidity" | awk '{print $2}' | tr -d '%')
        bme_pressure=$(echo "$bme_output" | grep "Pressure" | awk '{print $2}' | tr -d 'hPa')
    fi
    
    light=$(read_bh1750 2>/dev/null)
    [ "$light" = "error" ] && light="null"
    
    water_level=$(read_hcsr04t 2>/dev/null)
    if [ "$water_level" != "error" ] && [ -n "$water_level" ]; then
        water_percent=$(calc_water_percent $water_level)
        water_volume=$(calc_water_volume $water_percent)
    else
        water_level="null"
        water_percent="null"
        water_volume="null"
    fi
    
    # Build JSON
    cat > snapshot.json << EOF
{
  "timestamp": "$timestamp",
  "environment": {
    "temperature": $bme_temp,
    "humidity": $bme_humidity,
    "pressure": $bme_pressure,
    "light_lux": $light
  },
  "water_reservoir": {
    "distance_cm": $water_level,
    "percent_full": $water_percent,
    "volume_ml": $water_volume
  },
  "temperatures": {
EOF
    # Add temperature sensors
    first=true
    for dev in /sys/bus/w1/devices/28-*; do
        id=$(basename $dev)
        name=${TEMP_MAP[$id]}
        temp=$(read_temp $dev)
        
        if [ "$first" = true ]; then
            first=false
        else
            echo "," >> snapshot.json
        fi
        
        echo -n "    \"$name\": $temp" >> snapshot.json
    done
    cat >> snapshot.json << EOF
  },
  "soil_moisture": {
EOF
    # Add soil sensors
    first=true
    for chan in A0 A1 A2; do
        plant=${MOISTURE_MAP[$chan]}
        v=$(read_ads $chan)
        moisture=$(calc_moisture $plant $v)
        
        if [ "$first" = true ]; then
            first=false
        else
            echo "," >> snapshot.json
        fi
        
        cat >> snapshot.json << SOIL
    "$plant": {
      "voltage": $v,
      "percent": $moisture
    }
SOIL
    done
    cat >> snapshot.json << EOF
  }
}
EOF
    echo "Snapshot saved to snapshot.json"
}




echo ""
echo "===== HerbHub Sensor Snapshot ====="
echo ""
# Environmental Sensors
echo "Environment"
# BME280
bme_output=$(read_bme280 2>/dev/null)
if [ -n "$bme_output" ]; then
    echo "$bme_output" | while read line; do
        printf "  %s\n" "$line"
    done
fi
# BH1750 Light sensor
light=$(read_bh1750 2>/dev/null)
if [ "$light" != "error" ] && [ -n "$light" ]; then
    printf "  Light      : %s Lux\n" "$light"
fi
# HC-SR04T Water level sensor
water_level=$(read_hcsr04t 2>/dev/null)
if [ "$water_level" != "error" ] && [ -n "$water_level" ]; then
    if (( $(echo "$water_level < 400 && $water_level > 2" | bc -l) )); then
        water_percent=$(calc_water_percent $water_level)
        water_volume=$(calc_water_volume $water_percent)
        printf "  Water Level: %s cm (%s%% full, ~%s ml)\n" "$water_level" "$water_percent" "$water_volume"
    else
        printf "  Water Level: %s cm (out of range)\n" "$water_level"
    fi
fi
echo ""
echo "Temperatures (°C)"
for dev in /sys/bus/w1/devices/28-*; do
    id=$(basename $dev)
    name=${TEMP_MAP[$id]}
    temp=$(read_temp $dev)
    printf "%-10s : %s °C\n" "$name" "$temp"
done
echo ""
echo "Soil Moisture"
for chan in A0 A1 A2; do
    plant=${MOISTURE_MAP[$chan]}
    v=$(read_ads $chan)
    moisture=$(calc_moisture $plant $v)
    printf "%-10s : %.3f V  (%s%%)\n" "$plant" "$v" "$moisture"
done
echo ""
echo "writing snapshot json"
write_json_snapshot
