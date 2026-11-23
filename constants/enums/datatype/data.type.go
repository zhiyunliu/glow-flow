package datatype

type DataType int

const (
	String DataType = 0
	Int    DataType = 1
	Float  DataType = 2
	Bool   DataType = 3
	Object DataType = 4
	Array  DataType = 5
	Map    DataType = 6
	Any    DataType = 7
	Stream DataType = 8
)
