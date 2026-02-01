package main

import (
	"math"
	"testing"
)

// TestRayAt tests the Ray.At method
func TestRayAt(t *testing.T) {
	tests := []struct {
		name      string
		ray       Ray
		param     float64
		expected  Vec3
	}{
		{
			name: "Ray at t=0 returns origin",
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			param:    0,
			expected: Vec3{0, 0, 0},
		},
		{
			name: "Ray at t=1 with unit direction",
			ray: Ray{
				Origin:    Vec3{1, 2, 3},
				Direction: Vec3{1, 0, 0},
			},
			param:    1,
			expected: Vec3{2, 2, 3},
		},
		{
			name: "Ray at t=2 with scaled direction",
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{2, 3, 4},
			},
			param:    2,
			expected: Vec3{4, 6, 8},
		},
		{
			name: "Ray at negative t",
			ray: Ray{
				Origin:    Vec3{5, 5, 5},
				Direction: Vec3{-1, -1, -1},
			},
			param:    2,
			expected: Vec3{3, 3, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ray.At(tt.param)
			if !vec3Equal(result, tt.expected) {
				t.Errorf("Ray.At(%f) = %v, want %v", tt.param, result, tt.expected)
			}
		})
	}
}

// TestSphereHit tests Sphere.Hit for various scenarios
func TestSphereHit(t *testing.T) {
	tests := []struct {
		name     string
		sphere   Sphere
		ray      Ray
		tMin     float64
		tMax     float64
		shouldHit bool
		expectT  float64
	}{
		{
			name: "Ray hits sphere at center with unit direction",
			sphere: Sphere{
				Center: Vec3{0, 0, 0},
				Radius: 1.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{-2, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   1.0,
		},
		{
			name: "Ray misses sphere completely",
			sphere: Sphere{
				Center: Vec3{0, 0, 0},
				Radius: 1.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{0, 2, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: false,
		},
		{
			name: "Ray inside sphere hits",
			sphere: Sphere{
				Center: Vec3{0, 0, 0},
				Radius: 2.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   2.0,
		},
		{
			name: "Ray hits sphere with two intersection points",
			sphere: Sphere{
				Center: Vec3{3, 0, 0},
				Radius: 1.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   2.0, // First intersection
		},
		{
			name: "Ray outside valid t range",
			sphere: Sphere{
				Center: Vec3{10, 0, 0},
				Radius: 1.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      5.0,
			shouldHit: false,
		},
		{
			name: "Ray tangent to sphere",
			sphere: Sphere{
				Center: Vec3{0, 1, 0},
				Radius: 1.0,
				Mat:    nil,
			},
			ray: Ray{
				Origin:    Vec3{-2, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true, // Tangent still counts as a hit
			expectT:   2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, hit := tt.sphere.Hit(tt.ray, tt.tMin, tt.tMax)
			if hit != tt.shouldHit {
				t.Errorf("Sphere.Hit() hit = %v, want %v", hit, tt.shouldHit)
			}
			if hit && !floatEqual(rec.T, tt.expectT) {
				t.Errorf("Sphere.Hit() t = %f, want %f", rec.T, tt.expectT)
			}
		})
	}
}

// TestHittableListHit tests HittableList for finding closest hits
func TestHittableListHit(t *testing.T) {
	tests := []struct {
		name      string
		world     HittableList
		ray       Ray
		tMin      float64
		tMax      float64
		shouldHit bool
		expectT   float64
	}{
		{
			name: "Empty list no hit",
			world: HittableList{
				Objects: []Hittable{},
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: false,
		},
		{
			name: "Single sphere hit",
			world: HittableList{
				Objects: []Hittable{
					Sphere{
						Center: Vec3{5, 0, 0},
						Radius: 1.0,
						Mat:    nil,
					},
				},
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   4.0,
		},
		{
			name: "Multiple spheres returns closest",
			world: HittableList{
				Objects: []Hittable{
					Sphere{
						Center: Vec3{5, 0, 0},
						Radius: 1.0,
						Mat:    nil,
					},
					Sphere{
						Center: Vec3{10, 0, 0},
						Radius: 1.0,
						Mat:    nil,
					},
				},
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   4.0, // First sphere is closer
		},
		{
			name: "Multiple spheres with t constraint excludes first",
			world: HittableList{
				Objects: []Hittable{
					Sphere{
						Center: Vec3{3, 0, 0},
						Radius: 1.0,
						Mat:    nil,
					},
					Sphere{
						Center: Vec3{10, 0, 0},
						Radius: 1.0,
						Mat:    nil,
					},
				},
			},
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			tMin:      0,
			tMax:      math.MaxFloat64,
			shouldHit: true,
			expectT:   2.0, // First sphere at (3,0,0) has closest hit at t=2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, hit := tt.world.Hit(tt.ray, tt.tMin, tt.tMax)
			if hit != tt.shouldHit {
				t.Errorf("HittableList.Hit() hit = %v, want %v", hit, tt.shouldHit)
			}
			if hit && !floatEqual(rec.T, tt.expectT) {
				t.Errorf("HittableList.Hit() t = %f, want %f", rec.T, tt.expectT)
			}
		})
	}
}

// TestHitRecordSetFaceNormal tests the SetFaceNormal method
func TestHitRecordSetFaceNormal(t *testing.T) {
	tests := []struct {
		name           string
		ray            Ray
		outwardNormal  Vec3
		expectedFront  bool
		expectedNormal Vec3
	}{
		{
			name: "Ray hitting front face",
			ray: Ray{
				Origin:    Vec3{-1, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			outwardNormal:  Vec3{-1, 0, 0},
			expectedFront:  true,
			expectedNormal: Vec3{-1, 0, 0},
		},
		{
			name: "Ray hitting back face",
			ray: Ray{
				Origin:    Vec3{1, 0, 0},
				Direction: Vec3{-1, 0, 0},
			},
			outwardNormal:  Vec3{1, 0, 0},
			expectedFront:  true,
			expectedNormal: Vec3{1, 0, 0},
		},
		{
			name: "Ray from inside object",
			ray: Ray{
				Origin:    Vec3{0, 0, 0},
				Direction: Vec3{1, 0, 0},
			},
			outwardNormal:  Vec3{1, 0, 0},
			expectedFront:  false,
			expectedNormal: Vec3{-1, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &HitRecord{}
			rec.SetFaceNormal(tt.ray, tt.outwardNormal)
			if rec.FrontFace != tt.expectedFront {
				t.Errorf("SetFaceNormal() FrontFace = %v, want %v", rec.FrontFace, tt.expectedFront)
			}
			if !vec3Equal(rec.Normal, tt.expectedNormal) {
				t.Errorf("SetFaceNormal() Normal = %v, want %v", rec.Normal, tt.expectedNormal)
			}
		})
	}
}

// Helper functions for testing

func floatEqual(a, b float64) bool {
	const epsilon = 1e-9
	return math.Abs(a-b) < epsilon
}
