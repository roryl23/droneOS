package drone

import (
	"droneOS/internal/control/obstacle_avoidance"
	"droneOS/internal/control/pilot"
	"droneOS/internal/input/sensor/plugin/GT_U7"
	"droneOS/internal/input/sensor/plugin/HC_SR04"
	"droneOS/internal/input/sensor/plugin/MPU_6050"
	"droneOS/internal/input/sensor/plugin/frienda_obstacle_431S"
	"droneOS/internal/output/plugin/MG90S"
	"droneOS/internal/output/plugin/hawks_work_ESC"
)

var SensorFuncMap = map[string]interface{}{
	"frienda_obstacle_431S": frienda_obstacle_431S.Main,
	"GT_U7":                 GT_U7.Main,
	"HC_SR04":               HC_SR04.Main,
	"MPU_6050":              MPU_6050.Main,
}

var ControlFuncMap = map[string]interface{}{
	"obstacle_avoidance": obstacle_avoidance.Main,
	"pilot":              pilot.Main,
}

var OutputFuncMap = map[string]interface{}{
	"hawks_work_ESC": hawks_work_ESC.Main,
	"MG90S":          MG90S.Main,
}
