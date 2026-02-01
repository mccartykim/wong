package main

import (
	"math"
	"math/rand"
)

// Material interface defines how a material scatters light rays
type Material interface {
	Scatter(rIn Ray, rec HitRecord) (scattered Ray, attenuation Vec3, ok bool)
}

// Lambertian represents a diffuse material
type Lambertian struct {
	Albedo Vec3
}

// Scatter implements diffuse scattering for Lambertian materials
func (l Lambertian) Scatter(rIn Ray, rec HitRecord) (Ray, Vec3, bool) {
	// Random diffuse direction = normal + random unit vector
	scatterDirection := rec.Normal.Add(RandomUnitVector())

	// Handle edge case where random vector is opposite to normal
	if scatterDirection.Near_Zero() {
		scatterDirection = rec.Normal
	}

	scattered := Ray{
		Origin:    rec.P,
		Direction: scatterDirection,
	}
	attenuation := l.Albedo

	return scattered, attenuation, true
}

// Metal represents a reflective material
type Metal struct {
	Albedo Vec3
	Fuzz   float64
}

// Scatter implements reflective scattering for Metal materials
func (m Metal) Scatter(rIn Ray, rec HitRecord) (Ray, Vec3, bool) {
	// Reflect the ray around the normal
	reflected := rIn.Direction.UnitVector().Reflect(rec.Normal)

	// Add fuzz by adding a random vector in a unit sphere scaled by fuzz factor
	fuzzFactor := m.Fuzz
	if fuzzFactor > 1.0 {
		fuzzFactor = 1.0
	}
	reflected = reflected.Add(RandomInUnitSphere().Scale(fuzzFactor))

	scattered := Ray{
		Origin:    rec.P,
		Direction: reflected,
	}

	// Only scatter if the reflected ray is still going outward from the surface
	if scattered.Direction.Dot(rec.Normal) > 0 {
		return scattered, m.Albedo, true
	}

	return Ray{}, Vec3{}, false
}

// Dielectric represents a transparent/refracting material (glass)
type Dielectric struct {
	RefIdx float64 // Refractive index
}

// Scatter implements refractive/reflective scattering for Dielectric materials
func (d Dielectric) Scatter(rIn Ray, rec HitRecord) (Ray, Vec3, bool) {
	attenuation := Vec3{1.0, 1.0, 1.0} // Glass doesn't absorb light

	// Determine refraction ratio based on which side of the surface we're on
	var refractionRatio float64
	if rec.FrontFace {
		refractionRatio = 1.0 / d.RefIdx
	} else {
		refractionRatio = d.RefIdx
	}

	unitDirection := rIn.Direction.UnitVector()
	cosTheta := math.Min(1.0, unitDirection.Neg().Dot(rec.Normal))
	sinTheta := math.Sqrt(1.0 - cosTheta*cosTheta)

	// Check if refraction is possible using Snell's law
	cannotRefract := refractionRatio*sinTheta > 1.0

	var direction Vec3

	if cannotRefract || reflectance(cosTheta, refractionRatio) > rand.Float64() {
		// Total internal reflection or reflectance test says to reflect
		direction = unitDirection.Reflect(rec.Normal)
	} else {
		// Refraction is possible
		direction = unitDirection.Refract(rec.Normal, refractionRatio)
	}

	scattered := Ray{
		Origin:    rec.P,
		Direction: direction,
	}

	return scattered, attenuation, true
}

// reflectance calculates the reflectance using Schlick's approximation
// This determines the probability of reflection vs refraction at an interface
func reflectance(cosine, refIdx float64) float64 {
	// Schlick's approximation: R(θ) = R₀ + (1 - R₀) * (1 - cos θ)⁵
	// where R₀ = ((1 - n) / (1 + n))²
	r0 := (1 - refIdx) / (1 + refIdx)
	r0 = r0 * r0
	return r0 + (1-r0)*math.Pow(1-cosine, 5)
}
