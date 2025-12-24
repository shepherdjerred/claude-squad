package inspect

// Node represents a UI component in the inspection tree.
type Node struct {
	// Type is the component type (e.g., "List", "Menu", "ListItem").
	Type string `json:"type"`

	// ID is an optional identifier for the component.
	ID string `json:"id,omitempty"`

	// Bounds contains the component dimensions.
	Bounds Bounds `json:"bounds"`

	// Visible indicates if the component is currently rendered.
	Visible bool `json:"visible"`

	// State contains component-specific state information.
	State map[string]interface{} `json:"state,omitempty"`

	// Styles contains styling information.
	Styles *StyleInfo `json:"styles,omitempty"`

	// Children contains child components.
	Children []*Node `json:"children,omitempty"`

	// Content is the text content if applicable.
	Content string `json:"content,omitempty"`

	// Truncated contains truncation information if text was cut.
	Truncated *TruncationInfo `json:"truncated,omitempty"`
}

// Bounds represents component position and dimensions.
type Bounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// StyleInfo contains styling information for a component.
type StyleInfo struct {
	// Colors
	Foreground string `json:"foreground,omitempty"`
	Background string `json:"background,omitempty"`

	// Text decorations
	Bold      bool `json:"bold,omitempty"`
	Italic    bool `json:"italic,omitempty"`
	Underline bool `json:"underline,omitempty"`

	// Box model
	Border      string `json:"border,omitempty"`
	BorderColor string `json:"border_color,omitempty"`
	Padding     []int  `json:"padding,omitempty"` // [top, right, bottom, left]

	// Reference to named styles being used
	AppliedStyles []string `json:"applied_styles,omitempty"`
}

// TruncationInfo contains information about text truncation.
type TruncationInfo struct {
	// OriginalLength is the original text length before truncation.
	OriginalLength int `json:"original_length"`

	// DisplayLength is the displayed text length after truncation.
	DisplayLength int `json:"display_length"`

	// Ellipsis indicates if an ellipsis was added.
	Ellipsis bool `json:"ellipsis"`

	// OriginalText is the full original text (optional, for debugging).
	OriginalText string `json:"original_text,omitempty"`
}

// NewNode creates a new Node with the given type.
func NewNode(nodeType string) *Node {
	return &Node{
		Type:    nodeType,
		Visible: true,
		State:   make(map[string]interface{}),
	}
}

// WithID sets the node ID and returns the node for chaining.
func (n *Node) WithID(id string) *Node {
	n.ID = id
	return n
}

// WithBounds sets the node bounds and returns the node for chaining.
func (n *Node) WithBounds(x, y, width, height int) *Node {
	n.Bounds = Bounds{X: x, Y: y, Width: width, Height: height}
	return n
}

// WithState adds a state key-value pair and returns the node for chaining.
func (n *Node) WithState(key string, value interface{}) *Node {
	if n.State == nil {
		n.State = make(map[string]interface{})
	}
	n.State[key] = value
	return n
}

// WithStyles sets the node styles and returns the node for chaining.
func (n *Node) WithStyles(styles *StyleInfo) *Node {
	n.Styles = styles
	return n
}

// WithChildren sets the node children and returns the node for chaining.
func (n *Node) WithChildren(children []*Node) *Node {
	n.Children = children
	return n
}

// AddChild adds a child node and returns the parent for chaining.
func (n *Node) AddChild(child *Node) *Node {
	n.Children = append(n.Children, child)
	return n
}

// WithContent sets the node content and returns the node for chaining.
func (n *Node) WithContent(content string) *Node {
	n.Content = content
	return n
}

// WithTruncation sets truncation info and returns the node for chaining.
func (n *Node) WithTruncation(original, displayed int, hasEllipsis bool) *Node {
	n.Truncated = &TruncationInfo{
		OriginalLength: original,
		DisplayLength:  displayed,
		Ellipsis:       hasEllipsis,
	}
	return n
}
