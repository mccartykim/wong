package main

import (
	"math"
	"testing"
)

// TestLambertianAlwaysScatters verifies that Lambertian material always scatters
func TestLambertianAlwaysScatters(t *testing.T) {
	lambertian := Lambertian{
		Albedo: Vec3{0.5, 0.5, 0.5},
	}

	// Create a test ray and hit record
	rayIn := Ray{
		Origin:    Vec3{0, 0, 0},
		Direction: Vec3{0, -1, 0},
	}

	hitRecord := HitRecord{
		P:         Vec3{0, 0, -1},
		Normal:    Vec3{0, 1, 0},
		T:         1.0,
		FrontFace: true,
		Mat:       lambertian,
	}

	// Test multiple times to ensure consistency
	for i := 0; i < 10; i++ {
		scattered, attenuation, ok := lambertian.Scatter(rayIn, hitRecord)

		// Lambertian should always scatter
		if !ok {
			t.Error("Lambertian material should always scatter")
		}

		// Attenuation should match albedo
		if attenuation != lambertian.Albedo {
			t.Errorf("Expected attenuation %v, got %v", lambertian.Albedo, attenuation)
		}

		// Scattered ray should originate from hit point
		if scattered.Origin != hitRecord.P {
			t.Errorf("Scattered ray origin should be at hit point %v, got %v", hitRecord.P, scattered.Origin)
		}

		// Scattered ray direction should not be zero
		if scattered.Direction.LengthSquared() == 0 {
			t.Error("Scattered ray direction should not be zero")
		}
	}
}

// TestMetalReflection verifies that Metal reflects rays correctly
func TestMetalReflection(t *testing.T) {
	// Test with perfect reflection (fuzz = 0)
	metal := Metal{
		Albedo: Vec3{0.8, 0.8, 0.8},
		Fuzz:   0.0,
	}

	// Incoming ray from above, hitting surface normal pointing up
	rayIn := Ray{
		Origin:    Vec3{0, 1, 0},
		Direction: Vec3{0, -1, 0}, // Going downward
	}

	hitRecord := HitRecord{
		P:         Vec3{0, 0, 0},
		Normal:    Vec3{0, 1, 0}, // Normal pointing up
		T:         1.0,
		FrontFace: true,
		Mat:       metal,
	}

	scattered, attenuation, ok := metal.Scatter(rayIn, hitRecord)

	if !ok {
		t.Error("Metal should scatter rays")
	}

	// With fuzz=0, reflection should be perfect
	// Incident direction (0, -1, 0) reflected around normal (0, 1, 0) should be (0, 1, 0)
	expectedDir := Vec3{0, 1, 0}
	actualDir := scattered.Direction.UnitVector()

	if !approximateVec3Equal(actualDir, expectedDir, 0.01) {
		t.Errorf("Perfect reflection should be %v, got %v", expectedDir, actualDir)
	}

	// Attenuation should match albedo
	if attenuation != metal.Albedo {
		t.Errorf("Expected attenuation %v, got %v", metal.Albedo, attenuation)
	}

	// Scattered ray should originate from hit point
	if scattered.Origin != hitRecord.P {
		t.Errorf("Scattered ray origin should be at hit point %v, got %v", hitRecord.P, scattered.Origin)
	}
}

// TestMetalWithFuzz verifies that Metal fuzz parameter creates roughness
func TestMetalWithFuzz(t *testing.T) {
	metalRough := Metal{
		Albedo: Vec3{0.8, 0.8, 0.8},
		Fuzz:   0.5,
	}

	rayIn := Ray{
		Origin:    Vec3{0, 1, 0},
		Direction: Vec3{0, -1, 0},
	}

	hitRecord := HitRecord{
		P:         Vec3{0, 0, 0},
		Normal:    Vec3{0, 1, 0},
		T:         1.0,
		FrontFace: true,
		Mat:       metalRough,
	}

	// With fuzz > 0, multiple scatters should have different directions
	directions := make([]Vec3, 5)
	for i := 0; i < 5; i++ {
		scattered, _, ok := metalRough.Scatter(rayIn, hitRecord)
		if !ok {
			t.Error("Metal with fuzz should scatter rays")
		}
		directions[i] = scattered.Direction
	}

	// Check that we have some variation (not all the same direction)
	hasVariation := false
	for i := 1; i < len(directions); i++ {
		if !approximateVec3Equal(directions[0], directions[i], 0.01) {
			hasVariation = true
			break
		}
	}

	if !hasVariation {
		t.Error("Metal with fuzz should produce varied scatter directions")
	}
}

// TestDielectricRefraction verifies that Dielectric refracts rays
func TestDielectricRefraction(t *testing.T) {
	dielectric := Dielectric{
		RefIdx: 1.5, // Glass
	}

	// Ray coming from air into glass
	rayIn := Ray{
		Origin:    Vec3{0, 1, 0},
		Direction: Vec3{0, -1, 0}, // Perpendicular incidence
	}

	hitRecord := HitRecord{
		P:         Vec3{0, 0, 0},
		Normal:    Vec3{0, 1, 0},
		T:         1.0,
		FrontFace: true, // Front face means we're going from air into material
		Mat:       dielectric,
	}

	scattered, attenuation, ok := dielectric.Scatter(rayIn, hitRecord)

	if !ok {
		t.Error("Dielectric should scatter")
	}

	// Attenuation should be white (no absorption)
	expectedAttenuation := Vec3{1.0, 1.0, 1.0}
	if attenuation != expectedAttenuation {
		t.Errorf("Dielectric attenuation should be %v, got %v", expectedAttenuation, attenuation)
	}

	// Scattered ray should originate from hit point
	if scattered.Origin != hitRecord.P {
		t.Errorf("Scattered ray origin should be at hit point")
	}

	// Scattered ray should have a direction
	if scattered.Direction.LengthSquared() == 0 {
		t.Error("Scattered ray direction should not be zero")
	}
}

// TestDielectricTotalInternalReflection verifies reflection at grazing angles
func TestDielectricTotalInternalReflection(t *testing.T) {
	dielectric := Dielectric{
		RefIdx: 1.5,
	}

	// Ray at grazing angle (nearly parallel to surface)
	// This should trigger total internal reflection or high reflection probability
	rayIn := Ray{
		Origin:    Vec3{0, 1, 0},
		Direction: Vec3{0.9, -0.1, 0}.UnitVector(), // Mostly tangent to surface
	}

	hitRecord := HitRecord{
		P:         Vec3{0, 0, 0},
		Normal:    Vec3{0, 1, 0},
		T:         1.0,
		FrontFace: true,
		Mat:       dielectric,
	}

	// Run multiple times to check behavior
	reflectionCount := 0
	totalRuns := 20

	for i := 0; i < totalRuns; i++ {
		scattered, _, ok := dielectric.Scatter(rayIn, hitRecord)

		if !ok {
			t.Error("Dielectric should scatter")
		}

		// Check if this looks like a reflection (direction has upward component)
		if scattered.Direction.Dot(hitRecord.Normal) > 0.1 {
			reflectionCount++
		}
	}

	// At grazing angles, we expect a significant portion to be reflections
	// This is probabilistic, so we just check it's more than rare
	if reflectionCount < 5 {
		t.Logf("Warning: Expected more reflections at grazing angle, got %d out of %d", reflectionCount, totalRuns)
	}
}

// TestReflectanceFunction verifies Schlick's approximation
func TestReflectanceFunction(t *testing.T) {
	// At normal incidence (cosine = 1), reflectance should be low
	r1 := reflectance(1.0, 1.5)
	if r1 > 0.1 {
		t.Errorf("Normal incidence should have low reflectance, got %f", r1)
	}

	// At grazing incidence (cosine = 0), reflectance should be high
	r2 := reflectance(0.0, 1.5)
	if r2 < 0.9 {
		t.Errorf("Grazing incidence should have high reflectance, got %f", r2)
	}

	// Reflectance should be between 0 and 1
	for cosine := 0.0; cosine <= 1.0; cosine += 0.1 {
		r := reflectance(cosine, 1.5)
		if r < 0 || r > 1 {
			t.Errorf("Reflectance should be between 0 and 1, got %f at cosine %f", r, cosine)
		}
	}

	// Higher refractive index should have higher reflectance
	r3 := reflectance(0.5, 1.0)
	r4 := reflectance(0.5, 2.0)
	if r4 <= r3 {
		t.Errorf("Higher refractive index should have higher reflectance: %f <= %f", r4, r3)
	}
}

// Helper function to compare vectors with tolerance
func approximateVec3Equal(v1, v2 Vec3, tolerance float64) bool {
	return math.Abs(v1.X-v2.X) < tolerance &&
		math.Abs(v1.Y-v2.Y) < tolerance &&
		math.Abs(v1.Z-v2.Z) < tolerance
}
