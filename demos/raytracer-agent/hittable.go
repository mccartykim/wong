package main

import (
	"math"
)

// Ray represents a ray in 3D space
type Ray struct {
	Origin    Vec3
	Direction Vec3
}

// At returns the point on the ray at parameter t
// P(t) = Origin + t * Direction
func (r Ray) At(t float64) Vec3 {
	return r.Origin.Add(r.Direction.Scale(t))
}

// HitRecord contains information about a ray-object intersection
type HitRecord struct {
	P         Vec3        // Point of intersection
	Normal    Vec3        // Surface normal at intersection
	T         float64     // Parameter t where intersection occurred
	FrontFace bool        // True if ray hit from outside
	Mat       interface{} // Material at intersection point
}

// SetFaceNormal sets the normal and front-face flag based on ray and outward normal
// If the ray is hitting the front face, the normal points outward
// If the ray is hitting the back face, the normal points inward (opposite of outward normal)
func (hr *HitRecord) SetFaceNormal(r Ray, outwardNormal Vec3) {
	hr.FrontFace = r.Direction.Dot(outwardNormal) < 0
	if hr.FrontFace {
		hr.Normal = outwardNormal
	} else {
		hr.Normal = outwardNormal.Neg()
	}
}

// Hittable interface for objects that can be hit by a ray
type Hittable interface {
	Hit(r Ray, tMin, tMax float64) (HitRecord, bool)
}

// Sphere represents a sphere in 3D space
type Sphere struct {
	Center Vec3
	Radius float64
	Mat    interface{}
}

// Hit checks if a ray hits the sphere
// Uses the quadratic formula to solve: (P - C) · (P - C) = r²
// where P = O + t*D
func (s Sphere) Hit(r Ray, tMin, tMax float64) (HitRecord, bool) {
	oc := r.Origin.Sub(s.Center)
	a := r.Direction.LengthSquared()
	halfB := oc.Dot(r.Direction)
	c := oc.LengthSquared() - s.Radius*s.Radius

	discriminant := halfB*halfB - a*c
	if discriminant < 0 {
		return HitRecord{}, false
	}

	sqrtD := math.Sqrt(discriminant)

	// Find the nearest root that lies in the acceptable range
	root := (-halfB - sqrtD) / a
	if root < tMin || root > tMax {
		root = (-halfB + sqrtD) / a
		if root < tMin || root > tMax {
			return HitRecord{}, false
		}
	}

	rec := HitRecord{
		T:   root,
		P:   r.At(root),
		Mat: s.Mat,
	}

	outwardNormal := rec.P.Sub(s.Center).Scale(1.0 / s.Radius)
	rec.SetFaceNormal(r, outwardNormal)

	return rec, true
}

// HittableList contains a list of hittable objects
type HittableList struct {
	Objects []Hittable
}

// Hit finds the closest intersection with any object in the list
func (h HittableList) Hit(r Ray, tMin, tMax float64) (HitRecord, bool) {
	var closestRec HitRecord
	hitAnything := false
	closestSoFar := tMax

	for _, obj := range h.Objects {
		if rec, hit := obj.Hit(r, tMin, closestSoFar); hit {
			hitAnything = true
			closestSoFar = rec.T
			closestRec = rec
		}
	}

	return closestRec, hitAnything
}

// Add adds a hittable object to the list
func (h *HittableList) Add(obj Hittable) {
	h.Objects = append(h.Objects, obj)
}

// Clear removes all objects from the list
func (h *HittableList) Clear() {
	h.Objects = nil
}
