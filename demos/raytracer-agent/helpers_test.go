package main

import "math"

const epsilon = 1e-10

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func vec3Equal(a, b Vec3) bool {
	return almostEqual(a.X, b.X) && almostEqual(a.Y, b.Y) && almostEqual(a.Z, b.Z)
}
