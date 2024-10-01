import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import UserConfig from '../core/user.jsx';
import VeloTimestamp from "./time.jsx";
import ContextMenu from './context.jsx';
import JsonView from './json.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';

// Try to detect something that looks like a timestamp.
const timestamp_regex = /(\d{4}-[01]\d-[0-3]\dT[0-2]\d:[0-5]\d:[0-5]\d(?:\.\d+)?(?:[+-][0-2]\d:?[0-5]\d|Z))/;

// When the json object is larger than this many lines we offer to open it in its own dialog.
const maxSizeDialog = 50;

class ValueModal extends React.PureComponent {
    static propTypes = {
        value: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    render() {
        return <Modal show={true}
                      enforceFocus={true}
                      scrollable={false}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Body className="json-array-viewer">
                   <JsonView value={this.props.value}/>
                 </Modal.Body>
               </Modal>;
    }
}



export default class VeloValueRenderer extends React.Component {
    static contextType = UserConfig;
    static propTypes = {
        value: PropTypes.any,
        collapsed: PropTypes.bool,
    };

    // If the cell contains something that looks like a timestamp,
    // format it as such.
    maybeFormatTime = x => {
        let parts = x.split(timestamp_regex);
        return _.map(parts, (x, idx)=>{
            if (timestamp_regex.test(x)) {
                return <VeloTimestamp key={idx} iso={x} />;
            }
            return x;
        });
    }

    state = {
        showDialog: false,
    }

    estimateJsonSize = x=>{
        let serialized = JSON.stringify(x || "", null, " ");
        return serialized.split(/\n/).length;
    }

    render() {
        let v = this.props.value;

        if (_.isString(v)) {
            return <ContextMenu value={v}>{this.maybeFormatTime(v)}</ContextMenu>;
        }

        if (_.isNumber(v)) {
            return JSON.stringify(v);
        }

        if (_.isNull(v)) {
            return "";
        }

        let button = "";
        if (this.estimateJsonSize(v) > maxSizeDialog) {
            button = <button className="link"
                       onClick={x=>this.setState({showDialog:true})}>
                       <FontAwesomeIcon icon="plus"/>
                     </button>;
        }

        return <ContextMenu value={this.props.value}>
                 {button && <div>{ button }</div> }
                 <JsonView value={v} indent={0} collapsed={this.props.collapsed}/>
                 { this.state.showDialog &&
                   <ValueModal
                     onClose={x=>this.setState({showDialog:false})}
                     value={this.props.value}/> }
               </ContextMenu>;
    }
}
