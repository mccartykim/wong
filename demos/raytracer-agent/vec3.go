package main

import (
	"math"
	"math/rand"
)

// Vec3 represents a 3-dimensional vector
type Vec3 struct {
	X, Y, Z float64
}

// Add returns the sum of two vectors
func (v Vec3) Add(u Vec3) Vec3 {
	return Vec3{v.X + u.X, v.Y + u.Y, v.Z + u.Z}
}

// Sub returns the difference of two vectors (v - u)
func (v Vec3) Sub(u Vec3) Vec3 {
	return Vec3{v.X - u.X, v.Y - u.Y, v.Z - u.Z}
}

// Scale returns the vector scaled by a scalar value
func (v Vec3) Scale(t float64) Vec3 {
	return Vec3{v.X * t, v.Y * t, v.Z * t}
}

// MulVec returns the component-wise product of two vectors
func (v Vec3) MulVec(u Vec3) Vec3 {
	return Vec3{v.X * u.X, v.Y * u.Y, v.Z * u.Z}
}

// Dot returns the dot product of two vectors
func (v Vec3) Dot(u Vec3) float64 {
	return v.X*u.X + v.Y*u.Y + v.Z*u.Z
}

// Cross returns the cross product of two vectors
func (v Vec3) Cross(u Vec3) Vec3 {
	return Vec3{
		v.Y*u.Z - v.Z*u.Y,
		v.Z*u.X - v.X*u.Z,
		v.X*u.Y - v.Y*u.X,
	}
}

// Length returns the magnitude of the vector
func (v Vec3) Length() float64 {
	return math.Sqrt(v.LengthSquared())
}

// LengthSquared returns the squared magnitude of the vector
func (v Vec3) LengthSquared() float64 {
	return v.X*v.X + v.Y*v.Y + v.Z*v.Z
}

// UnitVector returns a normalized (unit) vector in the same direction
func (v Vec3) UnitVector() Vec3 {
	return v.Scale(1.0 / v.Length())
}

// Normalize is an alias for UnitVector - returns a normalized (unit) vector
func (v Vec3) Normalize() Vec3 {
	return v.UnitVector()
}

// Neg returns the negated vector
func (v Vec3) Neg() Vec3 {
	return Vec3{-v.X, -v.Y, -v.Z}
}

// Near_Zero checks if the vector is very close to zero
func (v Vec3) Near_Zero() bool {
	const s = 1e-8
	return math.Abs(v.X) < s && math.Abs(v.Y) < s && math.Abs(v.Z) < s
}

// Reflect returns the vector reflected around a normal
func (v Vec3) Reflect(normal Vec3) Vec3 {
	// r = v - 2(v · n)n
	return v.Sub(normal.Scale(2.0 * v.Dot(normal)))
}

// Refract returns the refracted vector using Snell's law
// v: incident vector (should be unit vector)
// normal: surface normal (should be unit vector)
// etaOverEtaPrime: ratio of refractive indices (eta / eta')
func (v Vec3) Refract(normal Vec3, etaOverEtaPrime float64) Vec3 {
	cosTheta := math.Min(1.0, v.Neg().Dot(normal))
	rOutPerp := v.Add(normal.Scale(cosTheta)).Scale(etaOverEtaPrime)
	rOutParallel := normal.Scale(-math.Sqrt(math.Abs(1.0 - rOutPerp.LengthSquared())))
	return rOutPerp.Add(rOutParallel)
}

// RandomVec3 generates a random vector with components in [0, 1)
func RandomVec3() Vec3 {
	return Vec3{rand.Float64(), rand.Float64(), rand.Float64()}
}

// RandomVec3InRange generates a random vector with components in [min, max)
func RandomVec3InRange(min, max float64) Vec3 {
	r := RandomVec3()
	return Vec3{
		min + (max-min)*r.X,
		min + (max-min)*r.Y,
		min + (max-min)*r.Z,
	}
}

// RandomInUnitSphere generates a random vector uniformly distributed in a unit sphere
func RandomInUnitSphere() Vec3 {
	for {
		p := RandomVec3InRange(-1, 1)
		if p.LengthSquared() < 1.0 {
			return p
		}
	}
}

// RandomUnitVector generates a random unit vector
func RandomUnitVector() Vec3 {
	return RandomInUnitSphere().UnitVector()
}

// RandomInUnitDisk generates a random vector uniformly distributed in a unit disk (z=0)
func RandomInUnitDisk() Vec3 {
	for {
		p := Vec3{
			2.0*rand.Float64() - 1.0,
			2.0*rand.Float64() - 1.0,
			0.0,
		}
		if p.LengthSquared() < 1.0 {
			return p
		}
	}
}
