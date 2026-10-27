package serialized

import "strings"

// commonTable is Unity's common type-tree string buffer: a node whose string
// offset has its high bit set indexes this, the offset being the string's
// position in the NUL-joined buffer. Read out of UnityPlayer.so of the build
// the bundles come from (6000.0.75f1), because the table is not the old
// public list with additions at the end - Unity 6 has strings in the middle
// of it - and every offset in a bundle depends on it.
var commonTable = []string{
	"AABB", "AnimationClip", "AnimationCurve", "AnimationState", "Array", "Base",
	"BitField", "bitset", "bool", "char", "ColorRGBA", "Component", "data",
	"deque", "double", "dynamic_array", "FastPropertyName", "first", "float",
	"Font", "GameObject", "Generic Mono", "GradientNEW", "GUID", "GUIStyle",
	"int", "list", "long long", "map", "Matrix4x4f", "MdFour", "MonoBehaviour",
	"MonoScript", "m_ByteSize", "m_Curve", "m_EditorClassIdentifier", "m_EditorHideFlags",
	"m_Enabled", "m_ExtensionPtr", "m_GameObject", "m_Index", "m_IsArray",
	"m_IsStatic", "m_MetaFlag", "m_Name", "m_ObjectHideFlags", "m_PrefabInternal",
	"m_PrefabParentObject", "m_Script", "m_StaticEditorFlags", "m_Type", "m_Version",
	"Object", "pair", "PPtr<Component>", "PPtr<GameObject>", "PPtr<Material>",
	"PPtr<MonoBehaviour>", "PPtr<MonoScript>", "PPtr<Object>", "PPtr<Prefab>",
	"PPtr<Sprite>", "PPtr<TextAsset>", "PPtr<Texture>", "PPtr<Texture2D>",
	"PPtr<Transform>", "Prefab", "Quaternionf", "Rectf", "RectInt", "RectOffset",
	"second", "set", "short", "size", "SInt16", "SInt32", "SInt64", "SInt8",
	"staticvector", "string", "TextAsset", "TextMesh", "Texture", "Texture2D",
	"Transform", "TypelessData", "UInt16", "UInt32", "UInt64", "UInt8", "unsigned int",
	"unsigned long long", "unsigned short", "vector", "Vector2f", "Vector3f",
	"Vector4f", "m_ScriptingClassIdentifier", "Gradient", "Type*", "int2_storage",
	"int3_storage", "BoundsInt", "m_CorrespondingSourceObject", "m_PrefabInstance",
	"m_PrefabAsset", "FileSize", "Hash128", "RenderingLayerMask", "fixed_array",
	"EntityId",
}

var commonBuffer = strings.Join(commonTable, "\x00") + "\x00"

func commonString(off uint32) string {
	if int(off) >= len(commonBuffer) {
		return ""
	}
	end := strings.IndexByte(commonBuffer[off:], 0)
	if end < 0 {
		return ""
	}
	return commonBuffer[off : int(off)+end]
}
