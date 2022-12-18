import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';

import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import SplitPane from 'react-split-pane';
import VeloValueRenderer from '../utils/value.jsx';

import TreeView from '../utils/tree/tree.jsx';

import './tree-cell.css';

const resizerStyle = {
//    width: "25px",
};


class TreeCellDialog extends Component {
    static propTypes = {
        name: PropTypes.string,
        data: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        data: {},
        cursor: {},
    }

    componentDidMount() {
        this.setState({data: this.props.data});
    }

    onToggle = (node, toggled)=>{
        let data = this.state.data;
        this.deactivateAll(data);
        node.active = true;
        if (node.children) {
            node.toggled = !node.toggled;
        }
        this.setState({cursor: node, data: Object.assign({}, data)});
    }

    deactivateAll = node=>{
        node.active = false;
        _.each(node.children, x=>this.deactivateAll(x));
    };

    toggleAll = (node, opened) => {
        node.toggled = opened;
        _.each(node.children, x=>this.toggleAll(x, opened));
    }

    render() {
        let data = {
            name: '',
            toggled: true,
            children: [],
        };

        if (_.isObject(this.state.data) && this.state.data.name) {
            data.children.push(this.state.data);
        }

        let value = this.state.cursor && this.state.cursor.data;

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={()=>this.props.onClose()}>
              <Modal.Header closeButton>
                {T(this.props.name)}
              </Modal.Header>
              <Modal.Body className="tree-modal-body">
                <SplitPane split="vertical"
                           defaultSize="40%"
                           resizerStyle={resizerStyle}>
                  <TreeView
                    data={data}
                    onSelect={this.onToggle}
                  />
                  <div className="tree-data-pane">
                    {value ?
                     <VeloValueRenderer value={value}/> :
                     <div>No Data</div>
                    }
                  </div>
                </SplitPane>
              </Modal.Body>
              <Modal.Footer>
                <Navbar className="w-100 justify-content-between">
                  <ButtonGroup className="float-right">
                    <Button variant="default"
                            onClick={()=>{
                                let top = this.state.data;
                                this.toggleAll(top, true);
                                this.setState({data: top});
                            }}>
                      <FontAwesomeIcon icon="folder-open"/>
                      <span className="button-label">{T("Open All")}</span>
                    </Button>

                    <Button variant="default"
                            onClick={()=>{
                                let top = this.state.data;
                                this.toggleAll(top, false);
                                this.setState({data: top});
                            }}>
                      <FontAwesomeIcon icon="folder"/>
                      <span className="button-label">{T("Close All")}</span>
                    </Button>
                  </ButtonGroup>
                </Navbar>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class TreeCell extends Component {
    static propTypes = {
        name: PropTypes.string,
        data: PropTypes.object,
    }

    state = {
        showDialog: false,
    }

    render() {
        return (
            <div>
              { this.state.showDialog &&
                <TreeCellDialog
                  name={this.props.name}
                  onClose={() => this.setState({showDialog: false})}
                  data={this.props.data}/>}

              {this.props.data && this.props.data.name &&
               <Button
                 onClick={()=>this.setState({showDialog: true})}
                 variant="default">
                 {this.props.data.name}
                 <span className="tree-button">
                   <FontAwesomeIcon icon="external-link-alt"/>
                 </span>
               </Button> }
            </div>
        );
    }
}
