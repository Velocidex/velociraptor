import "./hex.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Form from 'react-bootstrap/Form';
import ToolTip from '../widgets/tooltip.jsx';

export class HexViewDialog extends React.PureComponent {
    static propTypes = {
        data: PropTypes.string,
        byte_array: PropTypes.any,
        onClose: PropTypes.func.isRequired,
    }

    render() {
        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={true}
                   className="full-height"
                   scrollable={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Hex View data</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <div className="hex-dialog-body">
                  <HexView
                    byte_array={this.props.byte_array}
                    data={this.props.data}  height={50}/>
                </div>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

export class HexViewPopup extends React.Component {
    static propTypes = {
        data: PropTypes.string,
        byte_array: PropTypes.any,
    };

    state = {
        showDialog: false,
    }

    render() {
        if (!this.props.data  && !this.props.byte_array) {
            return <></>;
        }

        let string_data = _.toString(this.props.data);
        if (!string_data && this.props.byte_array) {
            for(let i=0;i<10 && i<this.props.byte_array.length;i++) {
                let c = this.props.byte_array[i];
                if (c>0x20 && c<0x7e) {
                    string_data += String.fromCharCode(c);
                } else {
                    string_data += ".";
                }
            }
        }
        if (string_data.length > 10) {
            string_data = string_data.substring(0, 10) + "...";
        }
        string_data = string_data.replace(/[^\x20-\x7E ]/g, '');
        return (
            <>
              { this.state.showDialog &&
                <HexViewDialog data={this.props.data}
                               byte_array={this.props.byte_array}
                               onClose={()=>this.setState({showDialog: false})}
                /> }
              <Button className="hex-popup client-link"
                      size="sm"
                      onClick={()=>this.setState({showDialog: true})}
                      variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                {string_data}
              </Button>
            </>
        );
    }
};



// A hex viewer suitable for small amountfs of text - No paging.
export default class HexView extends React.Component {
    static propTypes = {
        highlights: PropTypes.object,
        // Version of the highlights array to manage highligh updates
        highlight_version: PropTypes.number,
        base_offset: PropTypes.number,
        byte_array: PropTypes.any,
        // Version of the byte_array to manage updates of the data.
        version: PropTypes.any,
        data: PropTypes.string,
        height: PropTypes.number,
        max_height: PropTypes.number,
        setColumns: PropTypes.func,
        columns: PropTypes.number,
    };

    state = {
        hexDataRows: [],
        rows: 25,
        page: 0,
        expanded: false,
        hex_offset: false,
        highlights: {},
    }

    componentDidMount = () => {
        this.updateRepresentation();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.data, this.props.data) ||
            !_.isEqual(prevProps.version, this.props.version) ||
            !_.isEqual(prevProps.highlight_version, this.props.highlight_version) ||
            !_.isEqual(prevProps.base_offset, this.props.base_offset) ||
            !_.isEqual(prevProps.byte_array, this.props.byte_array) ||
            !_.isEqual(prevProps.highlights, this.props.highlights)) {
            this.updateRepresentation();
        }
    }

    updateRepresentation = () => {
        if (this.props.byte_array) {
            this.parseintArrayToHexRepresentation_(this.props.byte_array);
        } else {
            let utf8_encode = new TextEncoder().encode(this.props.data);
            this.parseintArrayToHexRepresentation_(utf8_encode);
        }
    }

    shouldHighlight = (offset)=>{
        if(_.isUndefined(this.props.highlights)) {
            return false;
        };

        // highlights is a map of key->name and value->a list of specs.
        for(const highlight of Object.values(this.props.highlights)) {
            for(const spec of highlight) {
                if (offset >= spec.start && offset < spec.end) {
                    return true;
                }
            }
        }
        return false;
    }

    // Populate the hex viewer from the byte_array prop
    parseintArrayToHexRepresentation_ = (intArray) => {
        if (!intArray) {
            intArray = "";
        }

        let hexDataRows = [];
        let columns = this.props.columns || 16;
        var chunkSize = this.state.rows * columns;
        let base_offset = this.props.base_offset || 0;
        let offset = this.state.page * chunkSize;

        for(var i = 0; i < this.state.rows; i++){
            var rowOffset = offset + (i * columns);
            var data = intArray.slice(i * columns, (i+1)*columns);
            var data_row = [];
            for (var j = 0; j < data.length; j++) {
                let char = data[j].toString(16);
                // add leading zero if necessary
                let text = ('0' + char).substr(-2);

                // Add a printable char for the text.
                let safe = ".";
                if (data[j] > 0x20 && data[j] < 0x7f) {
                    safe = String.fromCharCode(data[j]);
                };

                if (this.shouldHighlight(base_offset + rowOffset + j)) {
                    data_row.push({v: text, h: true, safe: safe});
                } else {
                    data_row.push({v: text, safe: safe});
                };
            };

            // Pad with extra spaces to maintain alignment
            if(data_row.length < columns) {
                let pad = columns - data_row.length % columns;
                for (let j = 0; j < pad; j++) {
                    data_row.push({v:" ", p:true, safe:" "});
                }
            }

            hexDataRows.push({
                offset: base_offset + rowOffset,
                data_row: data_row,
                data: data,
            });
        }

        this.setState({hexDataRows: hexDataRows, loading: false});
    };

    render() {
        let height = this.props.height || 5;
        let columns = this.props.columns || 16;
        let more = this.state.hexDataRows.length > height;
        let hexArea =
            <table className="hex-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <span key={"a"+ idx}></span>;
                    }
                    return <tr key={"a" + idx}>
                             <td>
                               { _.map(row.data_row, (x, idx)=>{
                                   let cname = "hex-char";
                                   if(x.h) {
                                       cname += " hex-highlight";
                                   } else if(x.p) {
                                        cname = "hex-padding";
                                   }
                                   return <span key={"aa"+ idx}
                                                className={cname}>
                                            { x.v }
                                          </span>;
                               })}
                             </td>
                           </tr>; })
                }
              </tbody>
            </table>;

        let contextArea =
            <table className="content-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <span key={"b" + idx}></span>;
                    }
                    return <tr key={idx}>
                             <td className="data">
                               { _.map(row.data_row, (x, idx)=>{
                                   let cname = "text-char";
                                   if(x.h) {
                                       cname += " hex-highlight";
                                   }
                                   return <span key={"bb"+ idx}
                                                className={cname}>
                                            { x.safe }
                                          </span>;
                               })}
                             </td>
                           </tr>;
                })}
              </tbody>
            </table>;

        return (
            <div className="panel hexdump">
              <div className="monospace hex-viewer">
                <table>
                  <thead>
                    <tr>
                      <th className="offset-area">
                        <ToolTip tooltip={T("Hex Offset")}>
                          <Button
                            variant="default-outline"
                            onClick={()=>this.setState({
                                hex_offset: !this.state.hex_offset
                            })}
                          >{T("Offset")}
                          </Button>
                        </ToolTip>
                      </th>
                      <th className="padding-area">
                        {_.map(_.range(0, columns), (x, idx)=>{
                            let x_str = x.toString(16);
                            x_str = ('0' + x_str).substr(-2);
                            return <span key={"d"+idx}
                                         className="hex-char">{x_str}</span>;
                        })}
                      </th>
                      <th>
                        { this.props.setColumns &&
                          <Form.Control as="select"
                                        style={{width: (1.22* this.props.columns) + "ex"}}
                                        className="hex-width-selector"
                                        placeholder={T("Width")}
                                        value={this.props.columns}
                                        onChange={e=>{
                                            this.props.setColumns(
                                                parseInt(e.target.value));
                                        }}
                          >
                            <option value="16">16</option>
                            <option value="24">24</option>
                            <option value="32">32</option>
                          </Form.Control>}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td >
                        <table className="offset-area">
                          <tbody>
                            { _.map(this.state.hexDataRows, (row, idx)=>{
                                if (idx >= height && !this.state.expanded) {
                                    return <div key={"c" + idx}></div>;
                                }
                                let offset = row.offset;
                                if (this.state.hex_offset) {
                                    offset = "0x" + offset.toString(16);
                                }
                                return <tr key={"c" + idx}>
                                         <td className="offset">
                                           { offset }
                                         </td>
                                       </tr>; })}
                          </tbody>
                        </table>
                      </td>
                      <td className="hex-container">
                        { hexArea }
                      </td>
                      <td className="context-container">
                        { contextArea }
                      </td>
                    </tr>
                    { more && (this.state.expanded ?
                               <tr>
                                 <td colSpan="16">
                                   <ToolTip tooltip={T("Collapse")}>
                                     <Button variant="default-outline"
                                             onClick={()=>this.setState({expanded: false})} >
                                       <i><FontAwesomeIcon icon="arrow-up"/></i>
                                     </Button>
                                   </ToolTip>
                                 </td>
                               </tr>
                               : <tr>
                                   <td colSpan="16">
                                     <ToolTip tooltip={T("Expand")}>
                                       <Button variant="default-outline"
                                               onClick={()=>this.setState({expanded: true})} >
                                         <i><FontAwesomeIcon icon="arrow-down"/></i>
                                       </Button>
                                     </ToolTip>
                                   </td>
                                 </tr>) }
                  </tbody>
                </table>
              </div>
            </div>
        );
    }
};
