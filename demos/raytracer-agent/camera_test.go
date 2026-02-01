package main

import (
	"math"
	"testing"
)


// TestCameraCenterRay verifies that a ray from the camera center goes through the image center
func TestCameraCenterRay(t *testing.T) {
	lookFrom := Vec3{0, 0, 0}
	lookAt := Vec3{0, 0, -1}
	vUp := Vec3{0, 1, 0}
	vFOV := 90.0
	aspectRatio := 1.0
	aperture := 0.0
	focusDist := 1.0
	
	camera := NewCamera(lookFrom, lookAt, vUp, vFOV, aspectRatio, aperture, focusDist)
	
	// Ray at the center of the image (s=0.5, t=0.5)
	ray := camera.GetRay(0.5, 0.5)
	
	// The ray origin should be at the camera position
	if dist(ray.Origin, lookFrom) > epsilon {
		t.Errorf("Ray origin mismatch: expected %v, got %v", lookFrom, ray.Origin)
	}
	
	// The ray direction should point roughly in the -Z direction (toward look-at)
	// Normalize to check direction
	dir := ray.Direction.UnitVector()
	expectedDir := Vec3{0, 0, -1}
	if dist(dir, expectedDir) > epsilon {
		t.Errorf("Ray direction mismatch: expected %v, got %v", expectedDir, dir)
	}
}

// TestCameraCornerRays verifies that rays at corners go to expected directions
func TestCameraCornerRays(t *testing.T) {
	lookFrom := Vec3{0, 0, 0}
	lookAt := Vec3{0, 0, -1}
	vUp := Vec3{0, 1, 0}
	vFOV := 90.0
	aspectRatio := 2.0 // width is 2x height
	aperture := 0.0
	focusDist := 1.0
	
	camera := NewCamera(lookFrom, lookAt, vUp, vFOV, aspectRatio, aperture, focusDist)
	
	// Test bottom-left corner (s=0, t=0)
	rayBL := camera.GetRay(0.0, 0.0)
	dirBL := rayBL.Direction.UnitVector()
	// Should point to lower-left (negative X, negative Y, negative Z)
	if dirBL.X > 0 {
		t.Errorf("Bottom-left ray X should be negative, got %v", dirBL.X)
	}
	if dirBL.Y > 0 {
		t.Errorf("Bottom-left ray Y should be negative, got %v", dirBL.Y)
	}
	if dirBL.Z > 0 {
		t.Errorf("Bottom-left ray Z should be negative, got %v", dirBL.Z)
	}
	
	// Test top-right corner (s=1, t=1)
	rayTR := camera.GetRay(1.0, 1.0)
	dirTR := rayTR.Direction.UnitVector()
	// Should point to upper-right (positive X, positive Y, negative Z)
	if dirTR.X < 0 {
		t.Errorf("Top-right ray X should be positive, got %v", dirTR.X)
	}
	if dirTR.Y < 0 {
		t.Errorf("Top-right ray Y should be positive, got %v", dirTR.Y)
	}
	if dirTR.Z > 0 {
		t.Errorf("Top-right ray Z should be negative, got %v", dirTR.Z)
	}
}

// TestCameraNoAperture verifies that with no aperture, rays originate exactly from camera position
func TestCameraNoAperture(t *testing.T) {
	lookFrom := Vec3{5, 3, 2}
	lookAt := Vec3{0, 0, 0}
	vUp := Vec3{0, 1, 0}
	vFOV := 60.0
	aspectRatio := 1.5
	aperture := 0.0
	focusDist := 10.0
	
	camera := NewCamera(lookFrom, lookAt, vUp, vFOV, aspectRatio, aperture, focusDist)
	
	// Multiple samples should all have the same origin (no aperture)
	expectedOrigin := lookFrom
	for sVal := 0.0; sVal <= 1.0; sVal += 0.25 {
		for tVal := 0.0; tVal <= 1.0; tVal += 0.25 {
			ray := camera.GetRay(sVal, tVal)
			if dist(ray.Origin, expectedOrigin) > epsilon {
				t.Errorf("Ray origin mismatch at (s=%f, t=%f): expected %v, got %v", sVal, tVal, expectedOrigin, ray.Origin)
			}
		}
	}
}

// Helper function to compute distance between two Vec3 points
func dist(a, b Vec3) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
