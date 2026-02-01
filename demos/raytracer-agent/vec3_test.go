package main

import (
	"math"
	"testing"
)




func TestVec3Add(t *testing.T) {
	v1 := Vec3{1, 2, 3}
	v2 := Vec3{4, 5, 6}
	result := v1.Add(v2)
	expected := Vec3{5, 7, 9}
	if !vec3Equal(result, expected) {
		t.Errorf("Add failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Sub(t *testing.T) {
	v1 := Vec3{5, 7, 9}
	v2 := Vec3{1, 2, 3}
	result := v1.Sub(v2)
	expected := Vec3{4, 5, 6}
	if !vec3Equal(result, expected) {
		t.Errorf("Sub failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Scale(t *testing.T) {
	v := Vec3{1, 2, 3}
	result := v.Scale(2.0)
	expected := Vec3{2, 4, 6}
	if !vec3Equal(result, expected) {
		t.Errorf("Scale failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3MulVec(t *testing.T) {
	v1 := Vec3{2, 3, 4}
	v2 := Vec3{5, 6, 7}
	result := v1.MulVec(v2)
	expected := Vec3{10, 18, 28}
	if !vec3Equal(result, expected) {
		t.Errorf("MulVec failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Dot(t *testing.T) {
	v1 := Vec3{1, 2, 3}
	v2 := Vec3{4, 5, 6}
	result := v1.Dot(v2)
	expected := 1.0*4.0 + 2.0*5.0 + 3.0*6.0 // 32
	if !almostEqual(result, expected) {
		t.Errorf("Dot failed: expected %f, got %f", expected, result)
	}
}

func TestVec3Cross(t *testing.T) {
	v1 := Vec3{1, 0, 0}
	v2 := Vec3{0, 1, 0}
	result := v1.Cross(v2)
	expected := Vec3{0, 0, 1}
	if !vec3Equal(result, expected) {
		t.Errorf("Cross failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Length(t *testing.T) {
	v := Vec3{3, 4, 0}
	result := v.Length()
	expected := 5.0
	if !almostEqual(result, expected) {
		t.Errorf("Length failed: expected %f, got %f", expected, result)
	}
}

func TestVec3LengthSquared(t *testing.T) {
	v := Vec3{3, 4, 0}
	result := v.LengthSquared()
	expected := 25.0
	if !almostEqual(result, expected) {
		t.Errorf("LengthSquared failed: expected %f, got %f", expected, result)
	}
}

func TestVec3UnitVector(t *testing.T) {
	v := Vec3{3, 4, 0}
	result := v.UnitVector()
	expectedLength := 1.0
	actualLength := result.Length()
	if !almostEqual(actualLength, expectedLength) {
		t.Errorf("UnitVector failed: expected length %f, got %f", expectedLength, actualLength)
	}
}

func TestVec3Normalize(t *testing.T) {
	v := Vec3{3, 4, 0}
	result := v.Normalize()
	expected := Vec3{0.6, 0.8, 0}
	if !vec3Equal(result, expected) {
		t.Errorf("Normalize failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Neg(t *testing.T) {
	v := Vec3{1, -2, 3}
	result := v.Neg()
	expected := Vec3{-1, 2, -3}
	if !vec3Equal(result, expected) {
		t.Errorf("Neg failed: expected %+v, got %+v", expected, result)
	}
}

func TestVec3Near_Zero(t *testing.T) {
	v1 := Vec3{1e-9, 1e-9, 1e-9}
	if !v1.Near_Zero() {
		t.Errorf("Near_Zero failed: %+v should be near zero", v1)
	}

	v2 := Vec3{0.1, 0, 0}
	if v2.Near_Zero() {
		t.Errorf("Near_Zero failed: %+v should not be near zero", v2)
	}
}

func TestVec3Reflect(t *testing.T) {
	// Reflect a vector off a surface normal
	v := Vec3{1, -1, 0}.Normalize() // incident vector
	normal := Vec3{0, 1, 0}          // normal pointing up
	result := v.Reflect(normal)
	
	// The reflected vector should have opposite y component
	if result.Y < 0 {
		t.Errorf("Reflect failed: reflected vector should have positive y component, got %+v", result)
	}
}

func TestVec3Refract(t *testing.T) {
	// Test refraction with eta ratio of 1.5 (glass)
	v := Vec3{1, -1, 0}.Normalize() // incident direction
	normal := Vec3{0, 1, 0}          // normal
	etaRatio := 1.0 / 1.5            // air to glass
	result := v.Refract(normal, etaRatio)
	
	// The refracted vector should exist (not NaN)
	if math.IsNaN(result.X) || math.IsNaN(result.Y) || math.IsNaN(result.Z) {
		t.Errorf("Refract failed: result contains NaN: %+v", result)
	}
}

func TestRandomVec3(t *testing.T) {
	v := RandomVec3()
	if v.X < 0 || v.X >= 1 || v.Y < 0 || v.Y >= 1 || v.Z < 0 || v.Z >= 1 {
		t.Errorf("RandomVec3 failed: values out of range [0, 1): %+v", v)
	}
}

func TestRandomVec3InRange(t *testing.T) {
	min := -5.0
	max := 5.0
	for i := 0; i < 10; i++ {
		v := RandomVec3InRange(min, max)
		if v.X < min || v.X >= max || v.Y < min || v.Y >= max || v.Z < min || v.Z >= max {
			t.Errorf("RandomVec3InRange failed: values out of range [%f, %f): %+v", min, max, v)
		}
	}
}

func TestRandomInUnitSphere(t *testing.T) {
	for i := 0; i < 10; i++ {
		v := RandomInUnitSphere()
		if v.LengthSquared() >= 1.0 {
			t.Errorf("RandomInUnitSphere failed: vector length squared %f is >= 1.0", v.LengthSquared())
		}
	}
}

func TestRandomUnitVector(t *testing.T) {
	for i := 0; i < 10; i++ {
		v := RandomUnitVector()
		length := v.Length()
		if !almostEqual(length, 1.0) {
			t.Errorf("RandomUnitVector failed: expected length 1.0, got %f", length)
		}
	}
}

func TestRandomInUnitDisk(t *testing.T) {
	for i := 0; i < 10; i++ {
		v := RandomInUnitDisk()
		if v.Z != 0 {
			t.Errorf("RandomInUnitDisk failed: Z should be 0, got %f", v.Z)
		}
		if v.LengthSquared() >= 1.0 {
			t.Errorf("RandomInUnitDisk failed: vector length squared %f is >= 1.0", v.LengthSquared())
		}
	}
}
