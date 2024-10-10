import _ from 'lodash';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import T from '../i8n/i8n.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import UserConfig from '../core/user.jsx';
import VeloForm from '../forms/form.jsx';
import Modal from 'react-bootstrap/Modal';
import Alert from 'react-bootstrap/Alert';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import ToolTip from '../widgets/tooltip.jsx';

const POLL_TIME = 5000;

class AddMetadataModal extends Component {
    static propTypes = {
        metadata: PropTypes.object,
        setMetadata: PropTypes.func,
        onClose: PropTypes.func.isRequired,
    };

    setMetadata = ()=>{
        let metadata = Object.assign({}, this.metadata);
        metadata[this.state.key] = this.state.value;
        this.props.setMetadata(metadata);
        this.props.onClose();
    }

    state = {
        key: "",
        value: "",
        error: "",
        disabled: true,
    }

    keyRegex = "^[a-zA-Z-0-9_ ]+$";

    setKey = key=>{
        // Key can not be the same as an existing key.
        if (!_.isEmpty(
            _.filter(this.props.metadata, (v, k)=> k === key))) {
            this.setState({
                disabled: true, key: key,
                error: T("Key can not be the same as another existing key")});
            return;
        }

        let re = new RegExp(this.keyRegex, "is");
        if (!re.test(key)) {
            this.setState({
                disabled: true, key: key,
                error: T("Key format not valid")});
            return;
        }
        this.setState({
            disabled: false, key: key,
            error: ""});
    }

    render() {
        return (
            <Modal size="lg" show={true}
                   onHide={this.props.onClose} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Add Metadata")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloForm param={{
                    name: T("Key"),
                    description: T("Key for new metadata"),
                    validating_regex: this.keyRegex,
                }}
                          value={this.state.key}
                          setValue={this.setKey}
                />
                <VeloForm param={{
                    name: T("Value"),
                    description: T("Value form new metadata")}}
                          value={this.state.value}
                          setValue={v=>this.setState({value: v})}
                />
                {this.state.error &&
                 <Alert variant="warning">
                   { this.state.error }
                 </Alert>}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        disabled={this.state.disabled}
                        onClick={this.setMetadata}>
                  {T("Yes do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class MetadataEditor extends Component {
    static contextType = UserConfig;

    static propTypes = {
        client_id: PropTypes.string,
        valueRenderer: PropTypes.func,
    }

    state = {
        metadata: {},
        custom_metadata: {},
        metadata_loading: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchMetadata, POLL_TIME);
        this.fetchMetadata();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.client_id !== prevProps.client_id) {
            this.fetchMetadata();
        }
    }

    fetchMetadata = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        let client_id = this.props.client_id;
        if (!client_id) {
            return;
        }
        this.setState({metadata_loading: true});

        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetClientMetadata/" + this.props.client_id,
                {}, this.source.token).then(response=>{
                    if (response.cancel) return;

                    let metadata = {};
                    _.each(response.data.items, x=>{
                        metadata[x.key] = x.value;
                    });

                    // Double buffer the metadata into custom_metadata
                    // to detect local changes.
                    var custom_metadata = this.state.custom_metadata;
                    if (_.isEmpty(custom_metadata)) {
                        custom_metadata = Object.assign({}, metadata);
                    }

                    this.setState({metadata: metadata,
                                   custom_metadata: custom_metadata,
                                   metadata_loading: false});
                });
    }

    setMetadata = metadata=>{
        let remove_keys = [];
        let add = [];

        _.each(metadata, (v, k)=>{
            if(!v) {
                remove_keys.push(k);
            } else {
                add.push({key: k, value: v});
            };
        });

        var params = {
            client_id: this.props.client_id,
            remove: remove_keys,
            add: add};
        api.post("v1/SetClientMetadata", params, this.source.token).then(() => {
            // Clear the custom_metadata to get the new set.
            this.setState({custom_metadata: {}});
            this.fetchMetadata();
        });
    }

    render() {
        let indexed = (this.context && this.context.traits &&
                       this.context.traits.customizations &&
                       this.context.traits.customizations.indexed_client_metadata) || [];

        let non_indexed = [];
        _.each(this.state.metadata, (v, k)=>{
            if (!_.includes(indexed, k)) {
                non_indexed.push(k);
            }
        });

        return <> {_.map(indexed, (x, i)=>{
            return <Row className="metadata-row" key={i}>
                     <Col sm="11">
                       <VeloForm key={i}
                                 param={{name: x}}
                                 value={this.state.custom_metadata[x]}
                                 setValue={v=>{
                                     let new_value = Object.assign(
                                         {}, this.state.custom_metadata);
                                     new_value[x] = v;
                                     this.setState({custom_metadata: new_value});
                                 }}
                       />
                     </Col>
                     <Col sm="1">
                       <ToolTip tooltip={T("Common metadata")}>
                         <Button variant="primary" disabled={true}>
                           <FontAwesomeIcon icon="trash"/>
                         </Button>
                       </ToolTip>
                     </Col>
                   </Row>;
        })}
                 {_.map(non_indexed, (x, i)=>{
                     return <Row className="metadata-row" key={i}>
                     <Col sm="11">
                       <VeloForm key={i}
                                 param={{name: x}}
                                 value={this.state.custom_metadata[x]}
                                 setValue={v=>{
                                     let new_value = Object.assign(
                                         {}, this.state.custom_metadata);
                                     new_value[x] = v;
                                     this.setState({custom_metadata: new_value});
                                 }}
                       />
                     </Col>
                     <Col sm="1">
                       <ToolTip tooltip={T("Clear")}>
                         <Button variant="primary"
                                 onClick={()=>{
                                     let new_value = Object.assign(
                                         {}, this.state.custom_metadata);
                                     new_value[x] = "";
                                     this.setMetadata(new_value);
                                 }}
                         >
                           <FontAwesomeIcon icon="trash"/>
                         </Button>
                       </ToolTip>
                     </Col>
                   </Row>;
        })}
                 <ButtonGroup>
                   <Button variant="default"
                           onClick={()=>this.setState({showAddDialog: true})}>
                     <FontAwesomeIcon icon="plus"/>
                   </Button>
                   <Button variant="default"
                           disabled={_.isEqual(this.state.custom_metadata,
                                               this.state.metadata)}
                           onClick={()=>this.setMetadata(
                               this.state.custom_metadata)}>
                     <FontAwesomeIcon icon="save"/>
                   </Button>
                 </ButtonGroup>
                 { this.state.showAddDialog &&
                   <AddMetadataModal
                     setMetadata={this.setMetadata}
                     onClose={()=>this.setState({showAddDialog: false})}
                     metadata={this.state.custom_metadata}
                   />}
               </>;
    }
}
