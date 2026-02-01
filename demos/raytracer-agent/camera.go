package main

import (
	"math"
)

// Camera represents a pinhole camera with depth-of-field via aperture
type Camera struct {
	origin        Vec3
	lowerLeftCorner Vec3
	horizontal    Vec3
	vertical      Vec3
	lensRadius    float64
	u             Vec3
	v             Vec3
	w             Vec3
}

// NewCamera creates a new Camera with the given parameters
// lookFrom: camera position
// lookAt: point the camera is looking at
// vUp: world up vector (typically 0,1,0)
// vFOV: vertical field of view in degrees
// aspectRatio: width/height ratio of the image
// aperture: diameter of the aperture (0 for no defocus blur)
// focusDist: distance to the plane in focus
func NewCamera(lookFrom, lookAt, vUp Vec3, vFOV, aspectRatio, aperture, focusDist float64) *Camera {
	// Convert vFOV from degrees to radians and compute viewport height
	theta := vFOV * math.Pi / 180
	h := math.Tan(theta / 2)
	
	// Compute orthonormal basis vectors
	w := lookFrom.Sub(lookAt).UnitVector()  // camera forward direction (away from look-at)
	u := vUp.Cross(w).UnitVector()           // camera right direction
	v := w.Cross(u)                          // camera up direction
	
	// Compute viewport dimensions
	viewportHeight := 2.0 * h
	viewportWidth := viewportHeight * aspectRatio
	
	// Compute world-space viewport vectors
	horizontal := u.Scale(viewportWidth * focusDist)
	vertical := v.Scale(viewportHeight * focusDist)
	
	// Compute lower-left corner of viewport
	lowerLeftCorner := lookFrom.Sub(horizontal.Scale(0.5)).Sub(vertical.Scale(0.5)).Sub(w.Scale(focusDist))
	
	// Lens radius for aperture
	lensRadius := aperture / 2.0
	
	return &Camera{
		origin:          lookFrom,
		lowerLeftCorner: lowerLeftCorner,
		horizontal:      horizontal,
		vertical:        vertical,
		lensRadius:      lensRadius,
		u:               u,
		v:               v,
		w:               w,
	}
}

// GetRay returns a ray from the camera through viewport coordinates (s, t)
// s and t should be in the range [0, 1] for the image bounds
// If aperture > 0, the ray origin is offset by a random point within the aperture disk
func (c *Camera) GetRay(s, t float64) Ray {
	// Random offset within the aperture disk for defocus blur
	rd := RandomInUnitDisk().Scale(c.lensRadius)
	offset := c.u.Scale(rd.X).Add(c.v.Scale(rd.Y))
	
	// Ray origin is camera position plus the offset
	rayOrigin := c.origin.Add(offset)
	
	// Ray direction points through the viewport
	// lowerLeftCorner + s*horizontal + t*vertical - origin - offset
	rayDirection := c.lowerLeftCorner.Add(c.horizontal.Scale(s)).Add(c.vertical.Scale(t)).Sub(c.origin).Sub(offset)
	
	return Ray{
		Origin:    rayOrigin,
		Direction: rayDirection,
	}
}
