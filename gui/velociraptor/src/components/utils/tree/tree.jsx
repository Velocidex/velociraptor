import './tree.css';

import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import classNames from "classnames";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


class Node extends Component {
    static propTypes = {
        node: PropTypes.object,
        onSelect: PropTypes.func,
    }

    getIcon = ()=>{
        let node = this.props.node;
        if (node.loading) {
            return <FontAwesomeIcon icon="spinner" spin/>;
        }

        let opened = node.toggled && node.known;

        if (opened) {
            return <FontAwesomeIcon icon="folder-open"/>;
        }
        return <FontAwesomeIcon icon="folder"/>;
    }

    render() {
        let node = this.props.node;
        if (_.isEmpty(node)) {
            return <></>;
        }

        return <ul>
                 <li
                   className={classNames({
                       isActive: node.active,
                       isRoot: !node.name,
                       })}
                   onClick={()=>this.props.onSelect(node)}>
                   <div className="tree-folder">
                     { this.getIcon() }
                     {node.name}
                   </div>
                  </li>
                 { node.toggled &&
                   _.map(node.children, (x, idx)=>{
                       return <Node
                                key={idx}
                                node={x}
                                onSelect={this.props.onSelect}
                              />;
                   })
                 }
               </ul>;
    }
}


export default class TreeView extends Component {
    static propTypes = {
        data: PropTypes.object,
        onSelect: PropTypes.func,
    }

    render() {
        return (
            <div className="file-tree">
              <Node
                node={this.props.data}
                onSelect={this.props.onSelect}
              />
            </div>
        );
    }
}
