package router

import "strings"

type trieNode struct {
	children   map[string]*trieNode
	paramChild *trieNode
	wildChild  *trieNode // ** catch-all
	route      *RouteConfig
	paramName  string
	isParam    bool
	isWildcard bool
}

type TrieRouter struct {
	root *trieNode
}

func NewTrieRouter() *TrieRouter {
	return &TrieRouter{root: &trieNode{children: make(map[string]*trieNode)}}
}

func (t *TrieRouter) Insert(route *RouteConfig) {
	parts := splitPath(route.Path)
	node := t.root

	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := part[1 : len(part)-1]
			if node.paramChild == nil {
				node.paramChild = &trieNode{isParam: true, paramName: name, children: make(map[string]*trieNode)}
			}
			node = node.paramChild
		} else if part == "**" {
			if node.wildChild == nil {
				node.wildChild = &trieNode{isWildcard: true, children: make(map[string]*trieNode)}
			}
			node = node.wildChild
		} else {
			if node.children == nil {
				node.children = make(map[string]*trieNode)
			}
			if _, ok := node.children[part]; !ok {
				node.children[part] = &trieNode{children: make(map[string]*trieNode)}
			}
			node = node.children[part]
		}
	}
	node.route = route
}

func (t *TrieRouter) Match(method, path string) *MatchedRoute {
	parts := splitPath(path)
	params := make(map[string]string)
	node := t.root

	for _, part := range parts {
		if child, ok := node.children[part]; ok {
			node = child
		} else if node.paramChild != nil {
			params[node.paramChild.paramName] = part
			node = node.paramChild
		} else if node.wildChild != nil {
			params["path"] = strings.Join(parts[strings.Index(joinPath(parts), part):], "/")
			node = node.wildChild
			break
		} else {
			return nil
		}
	}

	if node == nil || node.route == nil {
		return nil
	}

	// Check method match
	if len(node.route.Methods) > 0 {
		methodMatch := false
		for _, m := range node.route.Methods {
			if m == method || m == "*" {
				methodMatch = true
				break
			}
		}
		if !methodMatch {
			return nil
		}
	}

	return &MatchedRoute{Config: node.route, Params: params}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

func joinPath(parts []string) string {
	return "/" + strings.Join(parts, "/")
}

func (t *TrieRouter) Clear() {
	t.root = &trieNode{children: make(map[string]*trieNode)}
}
