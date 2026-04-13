package helper

import (
	"math"
)

/*
GetAbsolutePosition move the motor to absolute degree

returns
	move to position for driver
	destination position to communicate to clients to display the destination postion

definition

target position is 30 then motor will rotate to 30degree
irrespective where the current position is
based on getDestinationAngle() in machine_parser.js line# 420
*/
func GetAbsolutePosition(currentPos float64, targetPos float64, useShortestPath bool) (float64, float64) {
	var toMove float64
	if useShortestPath {
		toMove = getAbsolutePositionWithShortestPath(currentPos, targetPos)
	} else {
		toMove = getAbsolutePath(currentPos, targetPos)
	}
	destination := targetPos
	if destination > 0 {
		destination = math.Mod(destination, 360)
	} else {
		destination = math.Mod((destination + 360), 360)
	}
	return RoundFloatTo3(toMove), RoundFloatTo3(destination)
}

/*
getAbsolutePath get the angle to rotate in absolute path

for e.g. if current position is 50 and target position is 10 then rotate all the way
to 360 and go to 10
*/
func getAbsolutePath(currentPos float64, targetPos float64) float64 {
	toMove := math.Mod(currentPos, 360)
	toMove = toMove - targetPos
	toMove = 360 - toMove
	if math.Abs(toMove) > 360 {
		toMove = math.Mod(toMove, 360)
	}

	if targetPos < 0 {
		toMove = toMove - 360
		if toMove <= -360 {
			toMove = toMove + 360
		}
	}

	return toMove
}

/*
getAbsolutePositionWithShortestPath move to absolute postion but uses shortest path

for eg. if current position is 50 and target position is 40 then will -10 and move to 40
*/
func getAbsolutePositionWithShortestPath(currentPos float64, targetPos float64) float64 {
	currentPos = math.Mod(currentPos, 360)
	modeDiff := math.Mod((targetPos - currentPos), 360)
	shortestDistance := 180 - math.Abs(math.Abs(modeDiff)-180)

	test := math.Mod((modeDiff + 360), 360)
	if test < 180 {
		return shortestDistance * 1
	}
	return shortestDistance * -1
}

/*
GetRelativePosition returns the relative position based on the current position

for e.g. if the current position is 10 degree and ordered 20 degree then
motor will move to 30degree (10+20=30)
based on getDestinationAngle() in machine_parser.js line# 420
*/
func GetRelativePosition(currentPos float64, targetPos float64, prevDestinationAngle float64) (float64, float64) {
	currentPos = math.Mod(currentPos, 360)
	prevDestinationAngle = math.Mod(prevDestinationAngle, 360)
	var destination float64
	moveToPos := targetPos
	//at some point the current pos and prev pos shows a -1 difference
	//not sure how this difference is coming. In the previous code also doing this same workaround
	//refer getDestinationAngle() of node js code.
	if prevDestinationAngle == -1 {
		prevDestinationAngle = currentPos
	}
	if currentPos != prevDestinationAngle {
		diff1 := 360 - currentPos
		diff2 := math.Abs(prevDestinationAngle - currentPos)
		if diff1 > diff2 {
			moveToPos = prevDestinationAngle + targetPos - currentPos
		} else {
			moveToPos = (prevDestinationAngle + 360) + targetPos - currentPos
		}
	}
	destination = currentPos + destination
	if prevDestinationAngle >= 0 {
		destination = prevDestinationAngle + targetPos
	} else {
		destination = currentPos + targetPos
	}
	if destination > 0 {
		destination = math.Mod(destination, 360)
	} else {
		destination = math.Mod((destination + 360), 360)
	}
	return RoundFloatTo3(moveToPos), RoundFloatTo3(destination)
}
