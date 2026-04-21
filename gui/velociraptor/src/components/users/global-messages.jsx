import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import UserConfig from '../core/user.jsx';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Alert from 'react-bootstrap/Alert';
import VeloLog from "../widgets/logs.jsx";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloPagedTable from '../core/paged-table.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import ToolTip from '../widgets/tooltip.jsx';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

import "./global-messages.css";

class ShowGlobalMessages extends Component {
    static contextType = UserConfig;

    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }


    clearMessages = ()=>{
        api.post("v1/SetGUIOptions", {
            messages: -1,
        }, this.source.token).then((response) => {
            if (response.status === 200) {
                this.props.onClose();
            }
        });
    }

    getToolbar = ()=>{
        return <ButtonGroup>
                 <ToolTip tooltip={T("Clear Notifications")}>
                   <Button
                     className="btn btn-default"
                     onClick={this.clearMessages}
                   >
                     <FontAwesomeIcon icon="trash"/>
                   </Button>
                 </ToolTip>
               </ButtonGroup>;
    }

    render() {
        return (
            <Modal show={true}
                   dialogClassName="modal-70w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Global Messages")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloPagedTable
                  params={{
                      type: "USER_MESSAGES",
                  }}
                  toolbar={this.getToolbar()}
                />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={()=>this.props.onClose()}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.clearMessages}>
                  {T("Clear Messages")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


export default class GlobalMessages extends Component {
    static contextType = UserConfig;

    state = {
        show_messages: false,
    }

    render() {
        let msg = this.context.messages || [];
        return (
            <>
              <Button
                variant="primary"
                disabled={msg.length == 0}
                className={" danger global-messages"}
                onClick={()=>this.setState({show_messages: true})}
              >
                <FontAwesomeIcon icon="bell" />
              </Button>
              { this.state.show_messages &&
                <ShowGlobalMessages
                  onClose={()=>this.setState({show_messages: false})}
                /> }
            </>
        );
    }
}
