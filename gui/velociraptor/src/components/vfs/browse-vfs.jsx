import './browse-vfs.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import SplitPane from 'react-split-pane';

import VeloFileTree from './file-tree.jsx';
import VeloFileList from './file-list.jsx';
import VeloFileDetails from './file-details.jsx';


class VFSViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        updateCurrentNode: PropTypes.func.isRequired,
        node: PropTypes.object,
        vfs_path: PropTypes.array,
    }

    state = {
        topPaneSize: undefined,
        collapsed: false,
        vfs_path: [],
        current_node: {},
        current_selected_row: {},
    }

    collapse = () => {
        if (this.state.collapsed) {
            this.setState({topPaneSize: undefined, collapsed: false});
        } else {
            this.setState({topPaneSize: "10px", collapsed: true});
        }
    }

    updateVFSPath = (vfs_path) => {
        this.setState({vfs_path: vfs_path});
    };

    updateCurrentNode = (node) => {
        this.setState({current_node: node});
        this.props.updateCurrentNode(node);
    }

    updateCurrentSelectedRow = (row) => {
        this.setState({current_selected_row: Object.assign({_id: 1}, row)});
    }

    render() {
        return (
            <>
            <SplitPane split="vertical"
                       defaultSize="20%">
              <VeloFileTree client={this.props.client}
                            version={this.props.node && this.props.node.version}
                            className="file-tree"
                            updateVFSPath={this.updateVFSPath}
                            updateCurrentNode={this.updateCurrentNode}
                            vfs_path={this.state.vfs_path} />
              <SplitPane split="horizontal"
                         defaultSize="50%"
                         size={this.state.topPaneSize}
                         onResizerDoubleClick={this.collapse}>
                <VeloFileList
                  client={this.props.client}
                  updateCurrentNode={this.updateCurrentNode}
                  updateCurrentSelectedRow={this.updateCurrentSelectedRow}
                  version={this.props.node && this.props.node.version}
                  node={this.state.current_node} />
                <VeloFileDetails
                  client={this.props.client}
                  version={this.props.node && this.props.node.version}
                  updateCurrentNode={this.updateCurrentNode}
                  selectedRow={this.state.current_selected_row}
                  node={this.state.current_node} />
              </SplitPane>
            </SplitPane>
            </>
        );
    }
}

export default VFSViewer;
