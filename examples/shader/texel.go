// Copyright 2020 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore

//kage:unit pixel

package main

func Fragment(position vec4, texCoord vec2, color vec4) vec4 {
	pos := position.xy
	origin, size := imageDstRegionOnTexture()
	pos -= origin
	pos /= size
	return vec4(pos.x, pos.y, 0, 1)
}
